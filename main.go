package main

import (
	"crypto/md5"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// FileMD5 stores the path and its MD5 hash
type FileMD5 struct {
	Path string
	MD5  string
}

// getMD5 calculates the MD5 checksum of a file
func getMD5(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// processFile processes a single file
func processFile(srcPath, backupPath string, cMinutes int, conditionType string, resFile, errFile *os.File, wg *sync.WaitGroup, mu *sync.Mutex) {
	defer wg.Done()

	// Print the file being checked
	mu.Lock()
	fmt.Println("Checking:", srcPath)
	mu.Unlock()

	srcMD5, err := getMD5(srcPath)
	if err != nil {
		mu.Lock()
		_, _ = errFile.WriteString(fmt.Sprintf("Error calculating MD5 for source file: %s\n", srcPath))
		mu.Unlock()
		return
	}

	backupMD5, err := getMD5(backupPath)
	if err != nil {
		mu.Lock()
		_, _ = errFile.WriteString(fmt.Sprintf("File missing in backup: %s\n", backupPath))
		mu.Unlock()
	} else if srcMD5 != backupMD5 {
		mu.Lock()
		_, _ = errFile.WriteString(fmt.Sprintf("MD5 mismatch: %s\n", backupPath))
		mu.Unlock()
	} else {
		info, err := os.Stat(srcPath)
		if err != nil {
			mu.Lock()
			_, _ = errFile.WriteString(fmt.Sprintf("Error getting file info: %s\n", srcPath))
			mu.Unlock()
			return
		}

		var fileTime time.Time
		if conditionType == "access" {
			tmpStat := info.Sys().(*syscall.Stat_t)
			//macOS
			fileTime = time.Unix(tmpStat.Atimespec.Sec, tmpStat.Atimespec.Nsec)
			//Linux
			// fileTime = time.Unix(tmpStat.Atim.Sec, tmpStat.Atim.Nsec)
		} else {
			fileTime = info.ModTime()
		}

		if time.Since(fileTime).Minutes() > float64(cMinutes) {
			mu.Lock()
			_, _ = resFile.WriteString(fmt.Sprintf("%s\n", srcPath))
			mu.Unlock()
		}
	}
}

// compareDirs compares files in source and backup directories
func compareDirs(srcDir, backupDir string, cMinutes int, conditionType string, resFile, errFile *os.File) error {
	var wg sync.WaitGroup
	var mu sync.Mutex

	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		backupPath := filepath.Join(backupDir, relPath)
		wg.Add(1)
		go processFile(path, backupPath, cMinutes, conditionType, resFile, errFile, &wg, &mu)
		return nil
	})

	wg.Wait()
	return err
}

func main() {
	srcDir := flag.String("src", "", "Source directory")
	backupDir := flag.String("backup", "", "Backup directory")
	cMinutes := flag.Int("minutes", 0, "Time in minutes")
	conditionType := flag.String("type", "modify", "Condition type (modify/access)")

	flag.Parse()

	if *srcDir == "" || *backupDir == "" || *cMinutes == 0 {
		fmt.Println("Usage: go run main.go -src <source directory> -backup <backup directory> -minutes <time in minutes> -type <condition type>")
		return
	}

	resFile, err := os.OpenFile("res.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Error opening result file: %v\n", err)
		return
	}
	defer resFile.Close()

	errFile, err := os.OpenFile("error.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Error opening error file: %v\n", err)
		return
	}
	defer errFile.Close()

	err = compareDirs(*srcDir, *backupDir, *cMinutes, *conditionType, resFile, errFile)
	if err != nil {
		fmt.Printf("Error comparing directories: %v\n", err)
	} else {
		fmt.Println("Comparison complete. Check res.txt and error.txt for details.")
	}
}
