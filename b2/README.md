# b2

Native Backblaze B2 API client for Go. This module uses Backblaze's JSON and
upload APIs directly, not the S3-compatible API.

## Install

```bash
go get github.com/omniaura/go-files/b2
```

## Usage

```go
cl, err := b2.NewClient(ctx, b2.Options{
	KeyID:          keyID,
	ApplicationKey: appKey,
	BucketID:       bucketID,
	BucketName:     bucketName,
})
if err != nil {
	return err
}

file, err := cl.UploadFile(ctx, "users/123/avatar.jpg", contents, b2.UploadOptions{
	ContentType: "image/jpeg",
})
if err != nil {
	return err
}
fmt.Println(file.FileID)
```

`BucketID` and `BucketName` may be omitted when the application key is scoped to
one bucket and Backblaze returns those fields from `b2_authorize_account`.
