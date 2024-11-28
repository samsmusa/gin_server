package services

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/goccy/go-json"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func ParseShakaPackagerMetadata(rawMetadata string) (map[string]interface{}, error) {
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

type UploadFiles struct {
	VideoFile     *multipart.FileHeader   `form:"video"`
	AudioFiles    []*multipart.FileHeader `form:"audio"`
	AudioLang     []string                `form:"audio_lang"`
	SubtitleFiles []*multipart.FileHeader `form:"subtitles"`
}

func RunShakaPackager(c *gin.Context) {
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
	if !CreateTempDir(c, tempDir) {
		return
	}
	CleanupTempDir(tempDir)

	videoPath := filepath.Join(tempDir, form.VideoFile.Filename)
	if err := c.SaveUploadedFile(form.VideoFile, videoPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to save video file: %v", err),
		})
		return
	}

	var audioPaths []string
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

	var subtitlePaths []string
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
	fmt.Println(strings.Join(cmdArgs[:], " "))
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

func GetShakaPackagerInfo(c *gin.Context) {

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
		return
	}

	tempDir := "./temp"
	if !CreateTempDir(c, tempDir) {
		return
	}
	CleanupTempDir(tempDir)

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

	parsedMetadata, err := ParseShakaPackagerMetadata(string(output))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse metadata"})
		return
	}

	c.JSON(http.StatusOK, parsedMetadata)
}

func GetFileInfo(c *gin.Context, file *multipart.FileHeader) {

	tempDir := "./temp"
	if !CreateTempDir(c, tempDir) {
		return
	}
	CleanupTempDir(tempDir)

	tempFilePath := filepath.Join(tempDir, file.Filename)
	if err := c.SaveUploadedFile(file, tempFilePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save uploaded file"})
		return
	}

	cmd := exec.Command("ffprobe", "-v", "quiet", "-print_format", "json", "-show_streams", tempFilePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("FFprobe Error: %v, Output: %s", err, string(output))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to retrieve file metadata",
			"details": string(output),
		})
		return
	}

	var parsedMetadata map[string]interface{}
	if err := json.Unmarshal(output, &parsedMetadata); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse metadata"})
		return
	}
	fmt.Println(parsedMetadata)
	parsedMetadata["packaging_status"] = "completed successfully"

	c.JSON(http.StatusOK, parsedMetadata)
}

func ConvertToHLS(c *gin.Context, file *multipart.FileHeader) {
	tempDir := "./temp"
	if !CreateTempDir(c, tempDir) {
		return
	}
	defer CleanupTempDir(tempDir)

	tempFilePath := filepath.Join(tempDir, file.Filename)
	if err := c.SaveUploadedFile(file, tempFilePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save uploaded file"})
		return
	}

	cmd := exec.Command("ffprobe", "-v", "quiet", "-print_format", "json", "-show_streams", tempFilePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("FFprobe Error: %v, Output: %s", err, string(output))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to retrieve file metadata",
			"details": string(output),
		})
		return
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal(output, &metadata); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse metadata"})
		return
	}

	fmt.Println(metadata)

	streams, ok := metadata["streams"].([]interface{})
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "No streams found in metadata"})
		return
	}

	packagerCommand := []string{}
	outputFiles := []string{}

	for i, stream := range streams {
		streamMap, ok := stream.(map[string]interface{})
		if !ok {
			continue
		}

		streamType, ok := streamMap["codec_type"].(string)
		if !ok {
			continue
		}

		switch streamType {
		case "video":
			outputFile := filepath.Join(tempDir, fmt.Sprintf("video_%d.mp4", i))
			packagerCommand = append(packagerCommand, fmt.Sprintf("input=%s,stream=video,stream_selector=%d,output=%s", tempFilePath, i, outputFile))
			outputFiles = append(outputFiles, outputFile)

		case "audio":
			language := "und"
			if lang, exists := streamMap["tags"].(map[string]interface{})["language"]; exists {
				language, _ = lang.(string)
			}
			outputFile := filepath.Join(tempDir, fmt.Sprintf("audio_%s_%d.mp4", language, i))
			playlistName := fmt.Sprintf("audio_%s_%d.m3u8", language, i)
			packagerCommand = append(packagerCommand, fmt.Sprintf("input=%s,stream=audio,stream_selector=%d,language=%s,output=%s,playlist_name=%s", tempFilePath, i, language, outputFile, playlistName))
			outputFiles = append(outputFiles, outputFile)
		}
	}

	masterPlaylist := filepath.Join(tempDir, "master.m3u8")
	packagerCommand = append(packagerCommand, fmt.Sprintf("--hls_master_playlist_output=%s", masterPlaylist))
	packagerCommand = append(packagerCommand, "--segment_duration=6")

	fmt.Println(strings.Join(packagerCommand, " "))

	cmd = exec.Command("packager", packagerCommand...)
	packagerOutput, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Shaka Packager Error: %v, Output: %s", err, string(packagerOutput))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to convert file to HLS",
			"details": string(packagerOutput),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":          "HLS conversion completed successfully",
		"master_playlist":  masterPlaylist,
		"output_files":     outputFiles,
		"packager_command": packagerCommand,
		"packager_output":  string(packagerOutput),
	})
}
