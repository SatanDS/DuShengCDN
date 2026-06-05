package model

import (
	"dushengcdn/common"
	"errors"
	"gorm.io/gorm"
	"os"
	"path/filepath"
	"strings"
)

type File struct {
	Id              int    `json:"id"`
	Filename        string `json:"filename" gorm:"index"`
	Description     string `json:"description"`
	Uploader        string `json:"uploader"  gorm:"index"`
	UploaderId      int    `json:"uploader_id"  gorm:"index"`
	Link            string `json:"link" gorm:"unique;index"`
	UploadTime      string `json:"upload_time"`
	DownloadCounter int    `json:"download_counter"`
}

func GetAllFiles(startIdx int, num int) ([]*File, error) {
	var files []*File
	var err error
	err = DB.Order("id desc").Limit(num).Offset(startIdx).Find(&files).Error
	return files, err
}

func SearchFiles(keyword string) (files []*File, err error) {
	err = DB.Select([]string{"id", "filename", "description", "uploader", "uploader_id", "link", "upload_time", "download_counter"}).Where(
		"filename LIKE ? or uploader LIKE ? or uploader_id = ?", keyword+"%", keyword+"%", keyword).Find(&files).Error
	return files, err
}

func (file *File) Insert() error {
	var err error
	err = DB.Create(file).Error
	return err
}

func (file *File) Delete() error {
	filePath, err := safeUploadFilePath(file.Link)
	if err != nil {
		return err
	}
	if err = DB.Delete(file).Error; err != nil {
		return err
	}
	if err = os.Remove(filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func UpdateDownloadCounter(link string) {
	DB.Model(&File{}).Where("link = ?", link).UpdateColumn("download_counter", gorm.Expr("download_counter + 1"))
}

func safeUploadFilePath(link string) (string, error) {
	link = strings.TrimSpace(link)
	if link == "" || filepath.IsAbs(link) {
		return "", errors.New("invalid upload file link")
	}
	cleanLink := filepath.Clean(link)
	if cleanLink == "." || cleanLink == ".." || strings.HasPrefix(cleanLink, ".."+string(filepath.Separator)) {
		return "", errors.New("invalid upload file link")
	}
	baseDir, err := filepath.Abs(common.UploadPath)
	if err != nil {
		return "", err
	}
	filePath, err := filepath.Abs(filepath.Join(baseDir, cleanLink))
	if err != nil {
		return "", err
	}
	if filePath != baseDir && !strings.HasPrefix(filePath, baseDir+string(filepath.Separator)) {
		return "", errors.New("invalid upload file link")
	}
	return filePath, nil
}
