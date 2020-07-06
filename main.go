package main

import (
	"fmt"
	"github.com/dropbox/dropbox-sdk-go-unofficial/dropbox"
	"github.com/dropbox/dropbox-sdk-go-unofficial/dropbox/files"
	"github.com/dustin/go-humanize"
	"github.com/mitchellh/ioprogress"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func FileExists(name string) bool {
	if fi, err := os.Stat(name); err == nil {
		return fi.Mode().IsRegular()
	}
	return false
}

func DirExists(name string) bool {
	if fi, err := os.Stat(name); err == nil {
		return fi.Mode().IsDir()
	}
	return false
}

// Sends a get_metadata request for a given path and returns the response
func getFileMetadata(c files.Client, path string) (files.IsMetadata, error) {
	arg := files.NewGetMetadataArg(path)

	res, err := c.GetMetadata(arg)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func getRemoteListing(c files.Client, path string) (entries []files.IsMetadata, path_exists bool) {
	// return vals
	entries = make([]files.IsMetadata, 0)
	path_exists = false

	arg := files.NewListFolderArg(path)
	res, err := c.ListFolder(arg)
	if err != nil {
		xType := fmt.Sprintf("%T", err)
		fmt.Println("ERROR TYOE IS: ", xType)
		switch e := err.(type) {
		case files.ListFolderAPIError:
			if e.EndpointError.Path.Tag == files.LookupErrorNotFolder {
				var metaRes files.IsMetadata
				metaRes, err = getFileMetadata(c, path)
				entries = []files.IsMetadata{metaRes}
			}
		case dropbox.APIError:
			fmt.Println("DISNAE EXIST!")
		}

	} else {
		entries = res.Entries
		path_exists = true

		for res.HasMore {
			arg := files.NewListFolderContinueArg(res.Cursor)

			res, err = c.ListFolderContinue(arg)

			if err == nil {
				entries = append(entries, res.Entries...)
			}

		}
	}
	return
}

func createRemoteDir(c files.Client, dir_name string) (bool, error) {

	arg := files.NewCreateFolderArg(dir_name)
	if _, err := c.CreateFolderV2(arg); err != nil {
		return false, err
	}
	return true, nil
}

func getRecursiveFileEntries(startingDir string, fileNamesCollector []string) []string {

	files, err := ioutil.ReadDir(startingDir)
	if err != nil {
		log.Fatal(err)
	}

	for _, file := range files {
		if strings.HasPrefix(file.Name(), ".") {
			// ignore
		} else if file.IsDir() {
			fileNamesCollector = getRecursiveFileEntries(startingDir+"/"+file.Name(), fileNamesCollector)
		} else {
			fileNamesCollector = append(fileNamesCollector, startingDir+"/"+file.Name())
		}
	}
	return fileNamesCollector
}

func fileUpload(c files.Client, fileName string, destDir string) {
	fmt.Printf("UPLOADING \"%s\" TO \"%s\"\n", fileName, destDir)

	contents, err := os.Open(fileName)
	if err != nil {
		fmt.Println("OOFT! DUNNO WHERE YER FILE IS ", fileName)
		return
	}
	defer contents.Close()

	contentsInfo, err := contents.Stat()
	if err != nil {
		return
	}

	progressbar := &ioprogress.Reader{
		Reader: contents,
		DrawFunc: ioprogress.DrawTerminalf(os.Stderr, func(progress, total int64) string {
			return fmt.Sprintf("Uploading %s/%s",
				humanize.IBytes(uint64(progress)), humanize.IBytes(uint64(total)))
		}),
		Size: contentsInfo.Size(),
	}

	commitInfo := files.NewCommitInfo(destDir)
	commitInfo.Mode.Tag = "overwrite"

	// The Dropbox API only accepts timestamps in UTC with second precision.
	commitInfo.ClientModified = time.Now().UTC().Round(time.Second)

	//if contentsInfo.Size() > chunkSize {
	//	return uploadChunked(dbx, progressbar, commitInfo, contentsInfo.Size())
	//}

	if _, err = c.Upload(commitInfo, progressbar); err != nil {
		fmt.Println("OOFT ERROR! ", err)
		return
	}
	fmt.Println("DONE MOFO!\n")

}

func main() {
	fmt.Println("Syncb0ard!")

	args := os.Args
	if len(args) != 2 { // one is prog name, then one arg
		fmt.Fprintf(os.Stderr, "error: Need a directory name to sync\n")
		os.Exit(1)
	}

	dirName := args[1]

	if !DirExists(dirName) {
		fmt.Fprintf(os.Stderr, "error: Arg needs to be an existing directory name to sync\n")
		os.Exit(1)
	}

	fmt.Printf("Syncing %s\n", dirName)

	// TODO - flags -- daemon, verbose, dry-run

	key, err := ioutil.ReadFile("key.txt")
	if err != nil {
		log.Fatal(err)
	}
	token := strings.TrimSuffix(string(key), "\n")
	config := dropbox.Config{
		Token: token,
		//LogLevel: dropbox.LogInfo,
	}

	dbx := files.New(config)

	filesToUpload := []string{}
	filesToUpload = getRecursiveFileEntries(dirName, filesToUpload)

	baseDirSlice := strings.Split(dirName, "/")
	basePathFolderName := baseDirSlice[len(baseDirSlice)-1]
	basePathDestination := "/" + basePathFolderName
	fmt.Printf("BAse Path DEST is %s\n", basePathDestination)

	for _, fullFilePathName := range filesToUpload {
		fmt.Println()
		fullDirName := filepath.Dir(fullFilePathName)

		// If file is in subdirectory, let's build up that
		// dirpath and create as needed
		dirNameSlices := strings.Split(fullDirName, "/")
		destFolder := basePathDestination
		baseFound := false
		for _, n := range dirNameSlices {
			if n == basePathFolderName {
				baseFound = true
			} else if baseFound {
				destFolder += "/" + n
				_, path_exists := getRemoteListing(dbx, destFolder)
				if !path_exists {
					createRemoteDir(dbx, destFolder)
				}
			}
		}

		// destFolder now exists, upload file
		fileName := filepath.Base(fullFilePathName)
		remote_entries, path_exists := getRemoteListing(dbx, destFolder)
		if path_exists {
			fileFound := false
			for _, s := range remote_entries {

				metadata, ok := s.(*files.FileMetadata)
				if ok {
					if metadata.PathDisplay == fileName {
						fileFound = true
						fmt.Println("File ALready EXISTS!")
					}
				}
			}
			if !fileFound {
				fmt.Printf("FILE %s NOT FOUND - UPLOADINg to %s!\n", fileName, destFolder)
				fileUpload(dbx, fullFilePathName, destFolder)
			}
		}

	}

}
