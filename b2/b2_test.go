package b2

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewClientRequiresCredentials(t *testing.T) {
	t.Parallel()
	if _, err := NewClient(context.Background(), Options{}); err == nil {
		t.Fatal("expected missing credential error, got nil")
	}
	if _, err := NewClient(context.Background(), Options{KeyID: "id"}); err == nil {
		t.Fatal("expected missing application key error, got nil")
	}
}

func TestNewClientAuthorizesAndDefaultsBucket(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/b2api/v3/b2_authorize_account" {
			t.Fatalf("path = %s, want authorize", r.URL.Path)
		}
		id, key, ok := r.BasicAuth()
		if !ok || id != "key-id" || key != "app-key" {
			t.Fatalf("basic auth = %q/%q/%v", id, key, ok)
		}
		writeJSON(t, w, authorizeBody(serverURL(r), "bucket-id", "bucket-name"))
	}))
	defer server.Close()

	cl, err := NewClient(context.Background(), Options{
		KeyID:          "key-id",
		ApplicationKey: "app-key",
		AuthorizeURL:   server.URL + "/b2api/v3/b2_authorize_account",
		HTTPClient:     server.Client(),
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	auth := cl.Auth()
	if auth.APIURL != server.URL || auth.DownloadURL != server.URL || auth.BucketID != "bucket-id" || auth.BucketName != "bucket-name" {
		t.Fatalf("auth = %#v", auth)
	}
}

func TestUploadFileUsesNativeUploadAPI(t *testing.T) {
	t.Parallel()
	contents := []byte("hello b2")
	sum := sha1.Sum(contents)
	wantSHA := hex.EncodeToString(sum[:])

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/b2api/v3/b2_authorize_account":
			writeJSON(t, w, authorizeBody(serverURL(r), "bucket-id", "bucket-name"))
		case "/b2api/v3/b2_get_upload_url":
			if r.Header.Get("Authorization") != "account-token" {
				t.Fatalf("auth header = %q", r.Header.Get("Authorization"))
			}
			var body map[string]string
			decodeJSON(t, r, &body)
			if body["bucketId"] != "bucket-id" {
				t.Fatalf("bucketId = %q", body["bucketId"])
			}
			writeJSON(t, w, UploadURL{BucketID: "bucket-id", UploadURL: serverURL(r) + "/upload", AuthorizationToken: "upload-token"})
		case "/upload":
			if r.Header.Get("Authorization") != "upload-token" {
				t.Fatalf("upload auth = %q", r.Header.Get("Authorization"))
			}
			if r.Header.Get("X-Bz-File-Name") != "users%2F123%2Favatar.jpg" {
				t.Fatalf("file name header = %q", r.Header.Get("X-Bz-File-Name"))
			}
			if r.Header.Get("X-Bz-Content-Sha1") != wantSHA {
				t.Fatalf("sha = %q, want %q", r.Header.Get("X-Bz-Content-Sha1"), wantSHA)
			}
			if r.Header.Get("X-Bz-Info-origin") != "unit+test" {
				t.Fatalf("info header = %q", r.Header.Get("X-Bz-Info-origin"))
			}
			writeJSON(t, w, File{FileID: "file-id", FileName: "users/123/avatar.jpg", ContentSHA1: wantSHA, Size: int64(len(contents))})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	cl, err := NewClient(context.Background(), Options{
		KeyID:          "key-id",
		ApplicationKey: "app-key",
		AuthorizeURL:   server.URL + "/b2api/v3/b2_authorize_account",
		HTTPClient:     server.Client(),
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	file, err := cl.UploadFile(context.Background(), "users/123/avatar.jpg", contents, UploadOptions{Info: map[string]string{"origin": "unit test"}})
	if err != nil {
		t.Fatalf("UploadFile: %v", err)
	}
	if file.FileID != "file-id" || file.FileName != "users/123/avatar.jpg" {
		t.Fatalf("file = %#v", file)
	}
}

func TestListAndDeleteFileByName(t *testing.T) {
	t.Parallel()
	var deleted bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/b2api/v3/b2_authorize_account":
			writeJSON(t, w, authorizeBody(serverURL(r), "bucket-id", "bucket-name"))
		case "/b2api/v3/b2_list_file_names":
			var body map[string]any
			decodeJSON(t, r, &body)
			if body["prefix"] != "users/123/file.txt" || body["maxFileCount"].(float64) != 1 {
				t.Fatalf("list body = %#v", body)
			}
			writeJSON(t, w, ListResult{Files: []File{{FileID: "file-id", FileName: "users/123/file.txt"}}})
		case "/b2api/v3/b2_delete_file_version":
			var body map[string]string
			decodeJSON(t, r, &body)
			if body["fileName"] != "users/123/file.txt" || body["fileId"] != "file-id" {
				t.Fatalf("delete body = %#v", body)
			}
			deleted = true
			writeJSON(t, w, map[string]string{"fileId": "file-id", "fileName": "users/123/file.txt"})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	cl, err := NewClient(context.Background(), Options{KeyID: "key-id", ApplicationKey: "app-key", AuthorizeURL: server.URL + "/b2api/v3/b2_authorize_account", HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := cl.DeleteFileByName(context.Background(), "users/123/file.txt"); err != nil {
		t.Fatalf("DeleteFileByName: %v", err)
	}
	if !deleted {
		t.Fatal("delete endpoint was not called")
	}
}

func TestDownloadFileByName(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/b2api/v3/b2_authorize_account":
			writeJSON(t, w, authorizeBody(serverURL(r), "bucket-id", "bucket-name"))
		case "/file/bucket-name/folder/my file.txt":
			if r.Header.Get("Authorization") != "account-token" {
				t.Fatalf("download auth = %q", r.Header.Get("Authorization"))
			}
			_, _ = w.Write([]byte("downloaded"))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	cl, err := NewClient(context.Background(), Options{KeyID: "key-id", ApplicationKey: "app-key", AuthorizeURL: server.URL + "/b2api/v3/b2_authorize_account", HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	body, err := cl.DownloadFileByName(context.Background(), "folder/my file.txt")
	if err != nil {
		t.Fatalf("DownloadFileByName: %v", err)
	}
	if string(body) != "downloaded" {
		t.Fatalf("body = %q", body)
	}
}

func TestUploadInfoHeaderValidation(t *testing.T) {
	t.Parallel()
	if _, err := uploadInfoHeader("bad:key"); err == nil {
		t.Fatal("expected invalid header key error")
	}
	if _, err := uploadInfoHeader(strings.Repeat("a", 51)); err == nil {
		t.Fatal("expected long header key error")
	}
}

func authorizeBody(baseURL, bucketID, bucketName string) map[string]any {
	return map[string]any{
		"accountId":          "account-id",
		"authorizationToken": "account-token",
		"apiInfo": map[string]any{
			"storageApi": map[string]any{
				"apiUrl":      baseURL,
				"downloadUrl": baseURL,
				"bucketId":    bucketID,
				"bucketName":  bucketName,
			},
		},
	}
}

func serverURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode json: %v", err)
	}
}

func decodeJSON(t *testing.T, r *http.Request, value any) {
	t.Helper()
	if err := json.NewDecoder(r.Body).Decode(value); err != nil {
		t.Fatalf("decode json: %v", err)
	}
}
