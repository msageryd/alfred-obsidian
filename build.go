package main

import (
	"archive/zip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"howett.net/plist"
)

func main() {
	// Sync info.plist from installed workflow
	if err := syncInfoPlist(); err != nil {
		fmt.Printf("Error syncing info.plist: %v\n", err)
		os.Exit(1)
	}

	// 1. Increase version number
	plistPath := "info.plist"
	plistData := readPlist(plistPath)
	currentVersion, ok := plistData["version"].(string)
	if !ok {
		fmt.Println("Error: Could not find or parse version in info.plist")
		os.Exit(1)
	}
	newVersion := incrementVersion(currentVersion)
	plistData["version"] = newVersion
	writePlist(plistPath, plistData)

	// 2. Create build directory if it doesn't exist
	buildDir := "build"
	if err := os.MkdirAll(buildDir, os.ModePerm); err != nil {
		fmt.Printf("Error creating build directory: %v\n", err)
		os.Exit(1)
	}

	// 3. Create .alfredworkflow file in the build directory
	workflowName := fmt.Sprintf("Obsidian-msageryd-%s.alfredworkflow", newVersion)
	workflowPath := filepath.Join(buildDir, workflowName)
	createWorkflowZip(workflowPath)

	fmt.Printf("Version updated to %s and workflow file %s created.\n", newVersion, workflowPath)
	fmt.Println("To publish on GitHub, create a new release and upload the .alfredworkflow file from the build directory.")
}

func syncInfoPlist() error {
	// Get the Alfred preferences directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("error getting home directory: %v", err)
	}
	alfredPrefsDir := filepath.Join(homeDir, "Library", "CloudStorage", "Dropbox", "_system", "alfred5", "Alfred.alfredpreferences")

	// Find the workflow directory
	workflowsDir := filepath.Join(alfredPrefsDir, "workflows")
	var workflowDir string
	bundleID := "com.msageryd.obsidian" // Update this to match your workflow's bundle ID

	err = filepath.Walk(workflowsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && strings.HasPrefix(filepath.Base(path), "user.workflow.") {
			infoPlistPath := filepath.Join(path, "info.plist")
			if _, err := os.Stat(infoPlistPath); err == nil {
				plistData := readPlist(infoPlistPath)
				if plistData["bundleid"] == bundleID {
					workflowDir = path
					return filepath.SkipDir
				}
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("error finding workflow directory: %v", err)
	}
	if workflowDir == "" {
		return fmt.Errorf("workflow directory not found for bundle ID: %s", bundleID)
	}

	// Copy the info.plist from the workflow directory to the source directory
	srcInfoPlist := filepath.Join(workflowDir, "info.plist")
	destInfoPlist := "info.plist"
	input, err := ioutil.ReadFile(srcInfoPlist)
	if err != nil {
		return fmt.Errorf("error reading source info.plist: %v", err)
	}
	err = ioutil.WriteFile(destInfoPlist, input, 0644)
	if err != nil {
		return fmt.Errorf("error writing destination info.plist: %v", err)
	}

	fmt.Println("info.plist synced successfully")
	return nil
}

func readPlist(path string) map[string]interface{} {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		os.Exit(1)
	}

	var data map[string]interface{}
	_, err = plist.Unmarshal(content, &data)
	if err != nil {
		fmt.Printf("Error unmarshaling plist: %v\n", err)
		os.Exit(1)
	}

	return data
}

func writePlist(path string, data map[string]interface{}) {
	content, err := plist.Marshal(data, plist.XMLFormat)
	if err != nil {
		fmt.Printf("Error marshaling plist: %v\n", err)
		os.Exit(1)
	}

	err = ioutil.WriteFile(path, content, 0644)
	if err != nil {
		fmt.Printf("Error writing file: %v\n", err)
		os.Exit(1)
	}
}

func incrementVersion(version string) string {
	parts := strings.Split(version, ".")
	if len(parts) == 0 {
		return "1.0.0"
	}
	lastPart := parts[len(parts)-1]
	newLastPart := fmt.Sprintf("%d", atoi(lastPart)+1)
	parts[len(parts)-1] = newLastPart
	return strings.Join(parts, ".")
}

func createWorkflowZip(filename string) {
	zipFile, err := os.Create(filename)
	if err != nil {
		fmt.Printf("Error creating zip file: %v\n", err)
		os.Exit(1)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	err = filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip .git directory, build directory, build.go, and existing .alfredworkflow files
		if info.IsDir() && (info.Name() == ".git" || info.Name() == "build") {
			return filepath.SkipDir
		}
		if info.Name() == "build.go" || strings.HasSuffix(info.Name(), ".alfredworkflow") {
			return nil
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		header.Name = path
		if info.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}

		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			_, err = io.Copy(writer, file)
		}
		return err
	})

	if err != nil {
		fmt.Printf("Error creating workflow zip: %v\n", err)
		os.Exit(1)
	}
}

func atoi(s string) int {
	n := 0
	for _, ch := range s {
		ch -= '0'
		if ch > 9 {
			return 0
		}
		n = n*10 + int(ch)
	}
	return n
}