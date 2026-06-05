package service

import (
	"bytes"
	"mime/multipart"
	"strings"
	"testing"
)

func TestReadMultipartFileRejectsOversizedFile(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("cert", "oversized.pem")
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err = file.Write(bytes.Repeat([]byte("A"), int(maxTLSCertificateUploadBytes)+1)); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err = writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	reader := multipart.NewReader(&body, writer.Boundary())
	form, err := reader.ReadForm(maxTLSCertificateUploadBytes * 2)
	if err != nil {
		t.Fatalf("read multipart form: %v", err)
	}
	defer form.RemoveAll()

	files := form.File["cert"]
	if len(files) != 1 {
		t.Fatalf("expected one uploaded file, got %d", len(files))
	}
	_, err = readMultipartFile(files[0])
	if err == nil || !strings.Contains(err.Error(), "超过大小限制") {
		t.Fatalf("expected oversized upload error, got %v", err)
	}
}
