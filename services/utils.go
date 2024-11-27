package services

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"mime/multipart"
	"net/http"
	"os"
)

func CreateTempDir(c *gin.Context, dir string) bool {
	if err := os.MkdirAll(dir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to create temp directory: %v", err)})
		return false
	}
	return true
}
func SingleFileHandler(c *gin.Context) (*multipart.FileHeader, bool) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
		return nil, false
	}
	return file, true
}

func CleanupTempDir(dir string) {
	err := os.RemoveAll(dir)
	if err != nil {
		return
	}
}
