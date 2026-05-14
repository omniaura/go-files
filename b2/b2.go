// Package b2 implements a small native Backblaze B2 API client.
//
// It talks to the B2 native JSON/upload API directly. It does not use the
// Backblaze S3-compatible facade.
package b2

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
)

const (
	DefaultAuthorizeURL = "https://api.backblazeb2.com/b2api/v3/b2_authorize_account"
	apiPathPrefix       = "/b2api/v3/"
	maxUploadInfoKeyLen = 50
)

// Doer is the subset of *http.Client used by Client.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Options configures a native B2 client.
type Options struct {
	// KeyID is the Backblaze application key ID.
	KeyID string
	// ApplicationKey is the Backblaze application key secret.
	ApplicationKey string
	// BucketID is optional when the application key is scoped to a single bucket.
	BucketID string
	// BucketName is optional when the application key is scoped to a single bucket.
	BucketName string
	// AuthorizeURL overrides the default B2 authorization endpoint.
	AuthorizeURL string
	// HTTPClient overrides http.DefaultClient.
	HTTPClient Doer
}

// Client is a native B2 API client authorized for one account/key.
type Client struct {
	httpClient Doer
	authURL    string
	keyID      string
	appKey     string

	accountID          string
	authorizationToken string
	apiURL             string
	downloadURL        string
	bucketID           string
	bucketName         string
}

// Auth describes the account and bucket data returned by b2_authorize_account.
type Auth struct {
	AccountID          string
	AuthorizationToken string
	APIURL             string
	DownloadURL        string
	BucketID           string
	BucketName         string
}

// UploadOptions configures UploadFile.
type UploadOptions struct {
	ContentType string
	Info        map[string]string
}

// UploadURL is the result of b2_get_upload_url.
type UploadURL struct {
	BucketID           string `json:"bucketId"`
	UploadURL          string `json:"uploadUrl"`
	AuthorizationToken string `json:"authorizationToken"`
}

// File is a B2 file version record.
type File struct {
	FileID      string            `json:"fileId"`
	FileName    string            `json:"fileName"`
	AccountID   string            `json:"accountId,omitempty"`
	BucketID    string            `json:"bucketId,omitempty"`
	ContentType string            `json:"contentType,omitempty"`
	ContentSHA1 string            `json:"contentSha1,omitempty"`
	Size        int64             `json:"contentLength,omitempty"`
	Info        map[string]string `json:"fileInfo,omitempty"`
}

// ListOptions configures ListFileNames.
type ListOptions struct {
	Prefix        string
	StartFileName string
	MaxFileCount  int
	Delimiter     string
}

// ListResult is the result of b2_list_file_names.
type ListResult struct {
	Files        []File `json:"files"`
	NextFileName string `json:"nextFileName"`
}

type authorizeResponse struct {
	AccountID          string `json:"accountId"`
	AuthorizationToken string `json:"authorizationToken"`
	APIInfo            struct {
		StorageAPI struct {
			APIURL      string `json:"apiUrl"`
			DownloadURL string `json:"downloadUrl"`
			BucketID    string `json:"bucketId"`
			BucketName  string `json:"bucketName"`
		} `json:"storageApi"`
	} `json:"apiInfo"`
}

// NewClient authorizes an account and returns a native B2 client.
func NewClient(ctx context.Context, opts Options) (*Client, error) {
	if opts.KeyID == "" || opts.ApplicationKey == "" {
		return nil, fmt.Errorf("b2: KeyID and ApplicationKey are required")
	}
	authURL := opts.AuthorizeURL
	if authURL == "" {
		authURL = DefaultAuthorizeURL
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	cl := &Client{
		httpClient: httpClient,
		authURL:    authURL,
		keyID:      opts.KeyID,
		appKey:     opts.ApplicationKey,
		bucketID:   opts.BucketID,
		bucketName: opts.BucketName,
	}
	if err := cl.Authorize(ctx); err != nil {
		return nil, err
	}
	return cl, nil
}

// Authorize refreshes account authorization and bucket defaults.
func (cl *Client) Authorize(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cl.authURL, nil)
	if err != nil {
		return fmt.Errorf("b2: create authorize request: %w", err)
	}
	req.SetBasicAuth(cl.keyID, cl.appKey)

	var res authorizeResponse
	if err := cl.doJSON(req, &res); err != nil {
		return fmt.Errorf("b2: authorize account: %w", err)
	}
	storage := res.APIInfo.StorageAPI
	if res.AuthorizationToken == "" || storage.APIURL == "" || storage.DownloadURL == "" {
		return fmt.Errorf("b2: authorize account response missing required storage API fields")
	}

	cl.accountID = res.AccountID
	cl.authorizationToken = res.AuthorizationToken
	cl.apiURL = strings.TrimRight(storage.APIURL, "/")
	cl.downloadURL = strings.TrimRight(storage.DownloadURL, "/")
	if cl.bucketID == "" {
		cl.bucketID = storage.BucketID
	}
	if cl.bucketName == "" {
		cl.bucketName = storage.BucketName
	}
	return nil
}

// Auth returns the current authorization state.
func (cl *Client) Auth() Auth {
	return Auth{
		AccountID:          cl.accountID,
		AuthorizationToken: cl.authorizationToken,
		APIURL:             cl.apiURL,
		DownloadURL:        cl.downloadURL,
		BucketID:           cl.bucketID,
		BucketName:         cl.bucketName,
	}
}

// GetUploadURL requests a native B2 upload URL for the configured bucket.
func (cl *Client) GetUploadURL(ctx context.Context) (UploadURL, error) {
	if cl.bucketID == "" {
		return UploadURL{}, fmt.Errorf("b2: BucketID is required")
	}
	var out UploadURL
	err := cl.postAPI(ctx, "b2_get_upload_url", map[string]string{"bucketId": cl.bucketID}, &out)
	if err != nil {
		return UploadURL{}, err
	}
	if out.UploadURL == "" || out.AuthorizationToken == "" {
		return UploadURL{}, fmt.Errorf("b2: get upload url response missing upload fields")
	}
	return out, nil
}

// UploadFile uploads bytes through the B2 native upload API.
func (cl *Client) UploadFile(ctx context.Context, name string, contents []byte, opts UploadOptions) (File, error) {
	if strings.TrimSpace(name) == "" {
		return File{}, fmt.Errorf("b2: file name is required")
	}
	upload, err := cl.GetUploadURL(ctx)
	if err != nil {
		return File{}, err
	}
	return cl.UploadFileWithURL(ctx, upload, name, contents, opts)
}

// UploadFileWithURL uploads bytes using a previously fetched upload URL.
func (cl *Client) UploadFileWithURL(ctx context.Context, upload UploadURL, name string, contents []byte, opts UploadOptions) (File, error) {
	if upload.UploadURL == "" || upload.AuthorizationToken == "" {
		return File{}, fmt.Errorf("b2: upload URL and authorization token are required")
	}
	contentType := opts.ContentType
	if contentType == "" {
		contentType = contentTypeForName(name)
	}
	sum := sha1.Sum(contents)
	checksum := hex.EncodeToString(sum[:])

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upload.UploadURL, bytes.NewReader(contents))
	if err != nil {
		return File{}, fmt.Errorf("b2: create upload request: %w", err)
	}
	req.Header.Set("Authorization", upload.AuthorizationToken)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-Bz-File-Name", url.PathEscape(name))
	req.Header.Set("X-Bz-Content-Sha1", checksum)
	for k, v := range opts.Info {
		header, err := uploadInfoHeader(k)
		if err != nil {
			return File{}, err
		}
		req.Header.Set(header, url.QueryEscape(v))
	}

	var out File
	if err := cl.doJSON(req, &out); err != nil {
		return File{}, fmt.Errorf("b2: upload file %q: %w", name, err)
	}
	return out, nil
}

// ListFileNames lists current file versions in the configured bucket.
func (cl *Client) ListFileNames(ctx context.Context, opts ListOptions) (ListResult, error) {
	if cl.bucketID == "" {
		return ListResult{}, fmt.Errorf("b2: BucketID is required")
	}
	body := map[string]any{"bucketId": cl.bucketID}
	if opts.Prefix != "" {
		body["prefix"] = opts.Prefix
	}
	if opts.StartFileName != "" {
		body["startFileName"] = opts.StartFileName
	}
	if opts.MaxFileCount > 0 {
		body["maxFileCount"] = opts.MaxFileCount
	}
	if opts.Delimiter != "" {
		body["delimiter"] = opts.Delimiter
	}

	var out ListResult
	if err := cl.postAPI(ctx, "b2_list_file_names", body, &out); err != nil {
		return ListResult{}, err
	}
	return out, nil
}

// DeleteFileVersion deletes a specific B2 file version.
func (cl *Client) DeleteFileVersion(ctx context.Context, fileName, fileID string) error {
	if fileName == "" || fileID == "" {
		return fmt.Errorf("b2: fileName and fileID are required")
	}
	return cl.postAPI(ctx, "b2_delete_file_version", map[string]string{
		"fileName": fileName,
		"fileId":   fileID,
	}, nil)
}

// DeleteFileByName deletes the current file version matching name.
func (cl *Client) DeleteFileByName(ctx context.Context, name string) error {
	result, err := cl.ListFileNames(ctx, ListOptions{Prefix: name, MaxFileCount: 1})
	if err != nil {
		return err
	}
	if len(result.Files) == 0 || result.Files[0].FileName != name {
		return fmt.Errorf("b2: file %q not found", name)
	}
	return cl.DeleteFileVersion(ctx, result.Files[0].FileName, result.Files[0].FileID)
}

// DownloadFileByName downloads a file by name using the native download URL.
func (cl *Client) DownloadFileByName(ctx context.Context, name string) ([]byte, error) {
	if cl.bucketName == "" {
		return nil, fmt.Errorf("b2: BucketName is required")
	}
	if name == "" {
		return nil, fmt.Errorf("b2: file name is required")
	}
	downloadURL := cl.downloadURL + "/file/" + url.PathEscape(cl.bucketName) + "/" + escapeFileNamePath(name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("b2: create download request: %w", err)
	}
	req.Header.Set("Authorization", cl.authorizationToken)
	resp, err := cl.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("b2: download file %q: %w", name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, responseError(resp)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("b2: read download response: %w", err)
	}
	return body, nil
}

func (cl *Client) postAPI(ctx context.Context, apiName string, body any, out any) error {
	if cl.apiURL == "" || cl.authorizationToken == "" {
		return fmt.Errorf("b2: client is not authorized")
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("b2: encode %s request: %w", apiName, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cl.apiURL+apiPathPrefix+apiName, bytes.NewReader(buf))
	if err != nil {
		return fmt.Errorf("b2: create %s request: %w", apiName, err)
	}
	req.Header.Set("Authorization", cl.authorizationToken)
	req.Header.Set("Content-Type", "application/json")
	if err := cl.doJSON(req, out); err != nil {
		return fmt.Errorf("b2: %s: %w", apiName, err)
	}
	return nil
}

func (cl *Client) doJSON(req *http.Request, out any) error {
	resp, err := cl.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return responseError(resp)
	}
	if out == nil {
		_, err = io.Copy(io.Discard, resp.Body)
		return err
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func responseError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		msg = resp.Status
	}
	return fmt.Errorf("b2: unexpected status %d: %s", resp.StatusCode, msg)
}

func contentTypeForName(name string) string {
	if ext := filepath.Ext(name); ext != "" {
		if typ := mime.TypeByExtension(ext); typ != "" {
			return typ
		}
	}
	return "b2/x-auto"
}

func uploadInfoHeader(key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("b2: file info key is required")
	}
	if len(key) > maxUploadInfoKeyLen {
		return "", fmt.Errorf("b2: file info key %q exceeds %d characters", key, maxUploadInfoKeyLen)
	}
	if strings.ContainsAny(key, "\r\n:") {
		return "", fmt.Errorf("b2: invalid file info key %q", key)
	}
	return "X-Bz-Info-" + key, nil
}

func escapeFileNamePath(name string) string {
	parts := strings.Split(name, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}
