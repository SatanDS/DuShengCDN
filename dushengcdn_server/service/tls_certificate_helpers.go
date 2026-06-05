package service

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
)

const maxTLSCertificateUploadBytes int64 = 512 * 1024

func parseLeafCertificate(certPEM string) (*x509.Certificate, error) {
	certPEMBlock, _ := pem.Decode([]byte(certPEM))
	if certPEMBlock == nil {
		return nil, errors.New("证书 PEM 内容不合法")
	}
	leaf, err := x509.ParseCertificate(certPEMBlock.Bytes)
	if err != nil {
		return nil, err
	}
	return leaf, nil
}

func readMultipartFile(fileHeader *multipart.FileHeader) (string, error) {
	if fileHeader == nil {
		return "", errors.New("证书/密钥文件不能为空")
	}
	if fileHeader.Size > maxTLSCertificateUploadBytes {
		return "", fmt.Errorf("证书/密钥文件超过大小限制（最大 %d KB）", maxTLSCertificateUploadBytes/1024)
	}
	file, err := fileHeader.Open()
	if err != nil {
		return "", err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxTLSCertificateUploadBytes+1))
	if err != nil {
		return "", err
	}
	if int64(len(data)) > maxTLSCertificateUploadBytes {
		return "", fmt.Errorf("证书/密钥文件超过大小限制（最大 %d KB）", maxTLSCertificateUploadBytes/1024)
	}
	return string(data), nil
}
