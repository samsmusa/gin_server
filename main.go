package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"github.com/gin-gonic/gin"
)

type UploadFiles struct {
	VideoFile     *multipart.FileHeader   `form:"video"`
	AudioFiles    []*multipart.FileHeader `form:"audio"`
	AudioLang     []string                `form:"audio_lang"`
	SubtitleFiles []*multipart.FileHeader `form:"subtitles"`
}

func runShakaPackager(c *gin.Context) {
	var form UploadFiles


	if err := c.ShouldBind(&form); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Failed to bind form data and files.",
		})
		return
	}


	if form.VideoFile == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "No video file uploaded.",
		})
		return
	}


	tempDir := "./temp"
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to create temp directory: %v", err),
		})
		return
	}
	defer os.RemoveAll(tempDir)


	videoPath := filepath.Join(tempDir, form.VideoFile.Filename)
	if err := c.SaveUploadedFile(form.VideoFile, videoPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to save video file: %v", err),
		})
		return
	}


	audioPaths := []string{}
	for i, audioFile := range form.AudioFiles {
		audioPath := filepath.Join(tempDir, audioFile.Filename)
		if err := c.SaveUploadedFile(audioFile, audioPath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": fmt.Sprintf("Failed to save audio file %d: %v", i+1, err),
			})
			return
		}
		audioPaths = append(audioPaths, audioPath)
	}


	subtitlePaths := []string{}
	for i, subtitleFile := range form.SubtitleFiles {
		subtitlePath := filepath.Join(tempDir, subtitleFile.Filename)
		if err := c.SaveUploadedFile(subtitleFile, subtitlePath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": fmt.Sprintf("Failed to save subtitle file %d: %v", i+1, err),
			})
			return
		}
		subtitlePaths = append(subtitlePaths, subtitlePath)
	}


	cmdArgs := []string{
		"input=" + videoPath + ",stream=video,output=video_out.mp4",
	}


	for i, audioPath := range audioPaths {
		cmdArgs = append(cmdArgs, fmt.Sprintf(
			"input=%s,stream=audio,language=%s,output=audio_%d.mp4,playlist_name=audio_%d.m3u8",
			audioPath, form.AudioLang[i], i+1, i+1))
	}


	for i, subtitlePath := range subtitlePaths {
		cmdArgs = append(cmdArgs, fmt.Sprintf(
			"input=%s,stream=text,language=en,output=subtitle_%d.mp4,playlist_name=subtitle_%d.m3u8",
			subtitlePath, i+1, i+1))
	}


	cmdArgs = append(cmdArgs, "--hls_master_playlist_output", "master.m3u8", "--segment_duration", "6")


	cmd := exec.Command("packager", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to run Shaka Packager: %v", err),
		})
		return
	}


	c.JSON(http.StatusOK, gin.H{
		"message": "HLS packaging completed successfully.",
	})
}


func getShakaPackagerInfo(c *gin.Context) {
	
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
		return
	}

	
	tempDir, err := ioutil.TempDir("", "uploaded_files")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create temporary directory"})
		return
	}
	defer os.RemoveAll(tempDir)

	tempFilePath := filepath.Join(tempDir, file.Filename)
	if err := c.SaveUploadedFile(file, tempFilePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save uploaded file"})
		return
	}

	
	cmd := exec.Command("packager", "input="+tempFilePath, "--dump_stream_info")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Packager Error: %v, Output: %s", err, string(output))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to retrieve file metadata",
			"details": string(output),
		})
		return
	}

	
	parsedMetadata, err := parseShakaPackagerMetadata(string(output))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse metadata"})
		return
	}

	
	c.JSON(http.StatusOK, parsedMetadata)
}


func parseShakaPackagerMetadata(rawMetadata string) (map[string]interface{}, error) {
	lines := strings.Split(rawMetadata, "\n")
	var streams []map[string]interface{}
	packagingStatus := "unknown"

	for i, line := range lines {
		if strings.HasPrefix(line, "Stream [") {
			stream := make(map[string]interface{})
			streamType := strings.TrimSpace(strings.Split(line, "type:")[1])
			stream["type"] = streamType

			for j := i + 1; j < len(lines); j++ {
				innerLine := strings.TrimSpace(lines[j])
				if innerLine == "" || strings.HasPrefix(innerLine, "Stream [") {
					break
				}

				parts := strings.SplitN(innerLine, ":", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(parts[0])
					value := strings.TrimSpace(parts[1])
					stream[key] = value
				}
			}

			
			if durationStr, ok := stream["duration"].(string); ok {
				if duration, err := strconv.Atoi(durationStr); err == nil {
					timeScale, _ := strconv.Atoi(stream["time_scale"].(string))
					stream["duration_seconds"] = float64(duration) / float64(timeScale)
				}
			}

			streams = append(streams, stream)
		}

		if strings.Contains(line, "Packaging completed successfully") {
			packagingStatus = "completed successfully"
		}
	}

	return map[string]interface{}{
		"streams":          streams,
		"packaging_status": packagingStatus,
	}, nil
}

func convertMP3ToMP4(c *gin.Context) {

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "No MP3 file uploaded.",
		})
		return
	}


	if filepath.Ext(file.Filename) != ".mp3" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Uploaded file is not an MP3 file.",
		})
		return
	}


	tempDir := "./temp"
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to create temp directory: %v", err),
		})
		return
	}
	defer os.RemoveAll(tempDir)


	inputFilePath := filepath.Join(tempDir, file.Filename)
	if err := c.SaveUploadedFile(file, inputFilePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to save uploaded file: %v", err),
		})
		return
	}


	outputFilePath := filepath.Join(tempDir, file.Filename[:len(file.Filename)-len(filepath.Ext(file.Filename))]+"_converted.mp4")


	cmd := exec.Command("ffmpeg", "-i", inputFilePath, "-c:a", "aac", outputFilePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to convert MP3 to MP4: %v", err),
		})
		return
	}


	c.JSON(http.StatusOK, gin.H{
		"message":    "MP3 successfully converted to MP4.",
		"outputFile": outputFilePath,
	})
}

func convertMKVToMP4(c *gin.Context) {
	
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "No MKV file uploaded.",
		})
		return
	}

	
	if filepath.Ext(file.Filename) != ".mkv" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Uploaded file is not an MKV file.",
		})
		return
	}

	
	tempDir := "./temp"
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to create temp directory: %v", err),
		})
		return
	}
	defer os.RemoveAll(tempDir) 

	
	inputFilePath := filepath.Join(tempDir, file.Filename)
	if err := c.SaveUploadedFile(file, inputFilePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to save uploaded file: %v", err),
		})
		return
	}

	
	outputFilePath := filepath.Join(tempDir, file.Filename[:len(file.Filename)-len(filepath.Ext(file.Filename))]+"_converted.mp4")

	
	cmd := exec.Command("ffmpeg", "-i", inputFilePath, "-c:v", "copy", "-c:a", "aac", outputFilePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to convert MKV to MP4: %v", err),
		})
		return
	}

	
	c.JSON(http.StatusOK, gin.H{
		"message":    "MKV successfully converted to MP4.",
		"outputFile": outputFilePath,
	})
}

func main() {
	r := gin.Default()


	r.POST("/run-packager", runShakaPackager)
	r.POST("/meta", getShakaPackagerInfo)
	r.POST("/convert-mp3-to-mp4", convertMP3ToMP4)
	r.POST("/convert-mkv-to-mp4", convertMKVToMP4)


	log.Println("Server running on http://localhost:8080")
	r.Run(":8080")
}
