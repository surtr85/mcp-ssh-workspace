package sshclient

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ViewResult struct {
	Path        string `json:"path"`
	TotalLines  int    `json:"total_lines"`
	StartLine   int    `json:"start_line"`
	EndLine     int    `json:"end_line"`
	Content     string `json:"content"`
	IsTruncated bool   `json:"is_truncated"`
}

func (c *Client) ViewFile(remotePath string, startLine, endLine int, offset int, maxBytes int) (*ViewResult, error) {
	sftpCli, err := c.SFTP()
	if err != nil {
		return nil, err
	}

	remotePath = c.ResolvePath(remotePath)

	file, err := sftpCli.Open(remotePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open remote file: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat remote file: %w", err)
	}

	if stat.IsDir() {
		return nil, fmt.Errorf("path %s is a directory, not a file", remotePath)
	}

	if maxBytes <= 0 {
		maxBytes = 46080
	}

	// Read all lines
	scanner := bufio.NewScanner(file)
	// Allow large lines
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file content: %w", err)
	}

	totalLines := len(lines)
	if totalLines == 0 {
		return &ViewResult{
			Path:       remotePath,
			TotalLines: 0,
			StartLine:  1,
			EndLine:    0,
			Content:    "",
		}, nil
	}

	if startLine <= 0 {
		startLine = 1
	}
	if endLine <= 0 || endLine > totalLines {
		endLine = totalLines
	}
	if startLine > totalLines {
		return nil, fmt.Errorf("startLine (%d) exceeds total lines (%d)", startLine, totalLines)
	}

	var sb strings.Builder
	currentBytes := 0
	isTruncated := false

	actualEndLine := endLine
	for i := startLine - 1; i < endLine; i++ {
		lineNum := i + 1
		lineStr := fmt.Sprintf("%6d | %s\n", lineNum, lines[i])
		lineBytes := len(lineStr)

		if currentBytes+lineBytes > maxBytes {
			isTruncated = true
			actualEndLine = lineNum - 1
			break
		}

		sb.WriteString(lineStr)
		currentBytes += lineBytes
	}

	return &ViewResult{
		Path:        remotePath,
		TotalLines:  totalLines,
		StartLine:   startLine,
		EndLine:     actualEndLine,
		Content:     sb.String(),
		IsTruncated: isTruncated,
	}, nil
}

type ReplaceResult struct {
	Path         string `json:"path"`
	Replacements int    `json:"replacements"`
	TotalLines   int    `json:"total_lines"`
	Message      string `json:"message"`
}

func (c *Client) ReplaceFileContent(remotePath string, targetContent, replacementContent string, startLine, endLine int, allowMultiple bool) (*ReplaceResult, error) {
	sftpCli, err := c.SFTP()
	if err != nil {
		return nil, err
	}

	remotePath = c.ResolvePath(remotePath)

	file, err := sftpCli.Open(remotePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open remote file: %w", err)
	}
	defer file.Close()

	contentBytes, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read remote file: %w", err)
	}

	fullText := string(contentBytes)

	// If line ranges are specified, narrow down search scope
	var newText string
	var matchCount int

	if startLine > 0 || endLine > 0 {
		lines := strings.Split(fullText, "\n")
		totalLines := len(lines)
		if startLine <= 0 {
			startLine = 1
		}
		if endLine <= 0 || endLine > totalLines {
			endLine = totalLines
		}
		if startLine > totalLines {
			return nil, fmt.Errorf("startLine %d exceeds total lines %d", startLine, totalLines)
		}

		before := strings.Join(lines[:startLine-1], "\n")
		if len(before) > 0 {
			before += "\n"
		}

		targetChunk := strings.Join(lines[startLine-1:endLine], "\n")

		after := ""
		if endLine < totalLines {
			after = "\n" + strings.Join(lines[endLine:], "\n")
		}

		occurrences := strings.Count(targetChunk, targetContent)
		if occurrences == 0 {
			return nil, fmt.Errorf("target content not found within specified line range [%d, %d]", startLine, endLine)
		}
		if occurrences > 1 && !allowMultiple {
			return nil, fmt.Errorf("target content found %d times within range [%d, %d]; specify exact line range or set allowMultiple: true", occurrences, startLine, endLine)
		}

		var replacedChunk string
		if allowMultiple {
			replacedChunk = strings.ReplaceAll(targetChunk, targetContent, replacementContent)
			matchCount = occurrences
		} else {
			replacedChunk = strings.Replace(targetChunk, targetContent, replacementContent, 1)
			matchCount = 1
		}

		newText = before + replacedChunk + after
	} else {
		occurrences := strings.Count(fullText, targetContent)
		if occurrences == 0 {
			return nil, fmt.Errorf("target content not found in file")
		}
		if occurrences > 1 && !allowMultiple {
			return nil, fmt.Errorf("target content found %d times in file; specify line range or set allowMultiple: true", occurrences)
		}

		if allowMultiple {
			newText = strings.ReplaceAll(fullText, targetContent, replacementContent)
			matchCount = occurrences
		} else {
			newText = strings.Replace(fullText, targetContent, replacementContent, 1)
			matchCount = 1
		}
	}

	// Write back atomically
	tempRemote := remotePath + fmt.Sprintf(".tmp.%d", time.Now().UnixNano())
	tempFile, err := sftpCli.Create(tempRemote)
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary file for atomic write: %w", err)
	}

	_, err = io.WriteString(tempFile, newText)
	_ = tempFile.Close()
	if err != nil {
		_ = sftpCli.Remove(tempRemote)
		return nil, fmt.Errorf("failed to write content: %w", err)
	}

	// Replace original
	_ = sftpCli.Remove(remotePath)
	if err := sftpCli.Rename(tempRemote, remotePath); err != nil {
		// Fallback to direct write
		f, err := sftpCli.OpenFile(remotePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
		if err != nil {
			return nil, fmt.Errorf("failed to overwrite original file: %w", err)
		}
		defer f.Close()
		if _, err := io.WriteString(f, newText); err != nil {
			return nil, fmt.Errorf("failed to write file: %w", err)
		}
		_ = sftpCli.Remove(tempRemote)
	}

	newTotalLines := len(strings.Split(newText, "\n"))

	return &ReplaceResult{
		Path:         remotePath,
		Replacements: matchCount,
		TotalLines:   newTotalLines,
		Message:      fmt.Sprintf("Successfully replaced %d occurrence(s) in %s", matchCount, filepath.Base(remotePath)),
	}, nil
}

func (c *Client) WriteFile(remotePath string, content []byte, overwrite bool) error {
	sftpCli, err := c.SFTP()
	if err != nil {
		return err
	}

	remotePath = c.ResolvePath(remotePath)

	// Check existence if overwrite is false
	if !overwrite {
		if _, err := sftpCli.Stat(remotePath); err == nil {
			return fmt.Errorf("file %s already exists and overwrite is false", remotePath)
		}
	}

	// Ensure parent directories exist
	parentDir := filepath.Dir(remotePath)
	if err := c.mkdirAll(parentDir); err != nil {
		return fmt.Errorf("failed to create parent directories: %w", err)
	}

	file, err := sftpCli.OpenFile(remotePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return fmt.Errorf("failed to create/open file: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, bytes.NewReader(content)); err != nil {
		return fmt.Errorf("failed to write file content: %w", err)
	}

	return nil
}

type DirEntryInfo struct {
	Name    string `json:"name"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	ModTime string `json:"mod_time"`
}

func (c *Client) ListDir(remotePath string) ([]DirEntryInfo, error) {
	sftpCli, err := c.SFTP()
	if err != nil {
		return nil, err
	}

	remotePath = c.ResolvePath(remotePath)

	entries, err := sftpCli.ReadDir(remotePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", remotePath, err)
	}

	var results []DirEntryInfo
	for _, entry := range entries {
		results = append(results, DirEntryInfo{
			Name:    entry.Name(),
			IsDir:   entry.IsDir(),
			Size:    entry.Size(),
			Mode:    entry.Mode().String(),
			ModTime: entry.ModTime().Format(time.RFC3339),
		})
	}

	return results, nil
}

func (c *Client) mkdirAll(dirPath string) error {
	sftpCli, err := c.SFTP()
	if err != nil {
		return err
	}

	parts := strings.Split(filepath.Clean(dirPath), "/")
	current := ""
	if strings.HasPrefix(dirPath, "/") {
		current = "/"
	}

	for _, part := range parts {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		if stat, err := sftpCli.Stat(current); err != nil {
			if err := sftpCli.Mkdir(current); err != nil {
				// Verify if it was created concurrently
				if _, statErr := sftpCli.Stat(current); statErr != nil {
					return err
				}
			}
		} else if !stat.IsDir() {
			return fmt.Errorf("%s is not a directory", current)
		}
	}
	return nil
}

func (c *Client) ResolvePath(p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Clean(filepath.Join(c.GetCwd(), p))
}

func (c *Client) UploadFile(localPath, remotePath string, overwrite bool) (int64, error) {
	sftpCli, err := c.SFTP()
	if err != nil {
		return 0, err
	}

	srcFile, err := os.Open(localPath)
	if err != nil {
		return 0, fmt.Errorf("failed to open local file %s: %w", localPath, err)
	}
	defer srcFile.Close()

	remotePath = c.ResolvePath(remotePath)
	dir := filepath.Dir(remotePath)
	if err := c.mkdirAll(dir); err != nil {
		return 0, fmt.Errorf("failed to create remote directory %s: %w", dir, err)
	}

	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if !overwrite {
		flags = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	}

	dstFile, err := sftpCli.OpenFile(remotePath, flags)
	if err != nil {
		return 0, fmt.Errorf("failed to open remote file %s: %w", remotePath, err)
	}
	defer dstFile.Close()

	n, err := io.Copy(dstFile, srcFile)
	if err != nil {
		return n, fmt.Errorf("failed to upload file: %w", err)
	}
	return n, nil
}

func (c *Client) DownloadFile(remotePath, localPath string, overwrite bool) (int64, error) {
	sftpCli, err := c.SFTP()
	if err != nil {
		return 0, err
	}

	remotePath = c.ResolvePath(remotePath)
	srcFile, err := sftpCli.Open(remotePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open remote file %s: %w", remotePath, err)
	}
	defer srcFile.Close()

	localDir := filepath.Dir(localPath)
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return 0, fmt.Errorf("failed to create local directory %s: %w", localDir, err)
	}

	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if !overwrite {
		flags = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	}

	dstFile, err := os.OpenFile(localPath, flags, 0644)
	if err != nil {
		return 0, fmt.Errorf("failed to open local file %s: %w", localPath, err)
	}
	defer dstFile.Close()

	n, err := io.Copy(dstFile, srcFile)
	if err != nil {
		return n, fmt.Errorf("failed to download file: %w", err)
	}
	return n, nil
}
