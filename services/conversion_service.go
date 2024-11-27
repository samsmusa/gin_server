package services

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

func ConvertMP3ToMP4(c *gin.Context) {
	handleFileConversion(c, ".mp3", "_converted.mp4", []string{"-c:a", "aac"})
}

func ConvertMKVToMP4(c *gin.Context) {
	handleFileConversion(c, ".mkv", "_converted.mp4", []string{"-c:v", "copy", "-c:a", "aac"})
}

func handleFileConversion(c *gin.Context, expectedExt, outputSuffix string, ffmpegArgs []string) {

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("No %s file uploaded.", expectedExt)})
		return
	}

	if filepath.Ext(file.Filename) != expectedExt {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Uploaded file is not a %s file.", expectedExt)})
		return
	}

	tempDir := "./temp"
	if !CreateTempDir(c, tempDir) {
		return
	}
	CleanupTempDir(tempDir)

	inputFilePath := filepath.Join(tempDir, file.Filename)
	if err := c.SaveUploadedFile(file, inputFilePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to save uploaded file: %v", err)})
		return
	}

	outputFilePath := filepath.Join(tempDir, file.Filename[:len(file.Filename)-len(filepath.Ext(file.Filename))]+outputSuffix)

	cmdArgs := append([]string{"-i", inputFilePath}, ffmpegArgs...)
	cmdArgs = append(cmdArgs, outputFilePath)
	cmd := exec.Command("ffmpeg", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to convert %s: %v", expectedExt, err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    fmt.Sprintf("%s successfully converted.", expectedExt[1:]),
		"outputFile": outputFilePath,
	})
}
