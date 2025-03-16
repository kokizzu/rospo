package cmd

import (
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	pb "github.com/cheggaaa/pb/v3"
	"github.com/ferama/rospo/cmd/cmnflags"
	"github.com/ferama/rospo/pkg/logger"
	"github.com/ferama/rospo/pkg/sshc"
	"github.com/ferama/rospo/pkg/worker"
	"github.com/spf13/cobra"
)

var putLog = logger.NewLogger("[PUT ] ", logger.Magenta)

func init() {
	rootCmd.AddCommand(putCmd)

	cmnflags.AddSshClientFlags(putCmd.Flags())
	putCmd.Flags().IntP("max-workers", "w", 16, "nmber of parallel workers")
	putCmd.Flags().BoolP("recursive", "r", false, "if the copy should be recursive")

}

func putFile(sftpConn *sshc.SftpConnection, remote, localPath string, maxWorkers int) error {
	const chunkSize = 128 * 1024 // 128KB per chunk

	sftpConn.ReadyWait()

	remotePath, err := sftpConn.Client.RealPath(remote)
	if err != nil {
		return fmt.Errorf("invalid remote path: %s", remotePath)
	}
	putLog.Println("remotePath", remotePath)

	localStat, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("cannot stat local path: %s", localPath)
	}
	fileSize := localStat.Size()

	lFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("cannot open local file: %s", err)
	}
	defer lFile.Close()

	// Check if remote file already exists and determine resume offset
	var offset int64 = 0
	if remoteStat, err := sftpConn.Client.Stat(remotePath); err == nil {
		offset = remoteStat.Size()
	}

	if offset >= fileSize {
		putLog.Println("File already fully uploaded.")
		return nil
	}

	progressCh := make(chan int64, maxWorkers)

	go func() {
		tmpl := `{{string . "target" | white}} {{counters . | blue }} {{bar . "|" "=" ">" "." "|" }} {{percent . | blue }} {{speed . | blue }} {{rtime . "ETA %s" | blue }}`
		pbar := pb.ProgressBarTemplate(tmpl).Start64(fileSize)
		pbar.Set(pb.Bytes, true)
		pbar.Set(pb.SIBytesPrefix, true)
		pbar.Set("target", filepath.Base(remotePath))
		pbar.Add64(offset)
		for w := range progressCh {
			pbar.Add64(w)
		}
		pbar.Finish()
	}()

	putLog.Printf("Using %d workers", maxWorkers)
	workerPool := worker.NewPool(maxWorkers)
	defer workerPool.Stop()

	// Enqueue only the remaining chunks for workers
	for chunkOffset := offset; chunkOffset < fileSize; chunkOffset += chunkSize {
		workerPool.Enqueue(func() {
			for {
				err := uploadChunk(sftpConn, remotePath, lFile, chunkOffset, chunkSize, progressCh)
				if err == nil {
					break // Success, move to next chunk
				}
			}
		})
	}

	workerPool.Wait()
	close(progressCh)

	return sftpConn.Client.Chmod(remotePath, localStat.Mode())
}

// Upload Chunk
func uploadChunk(sftpConn *sshc.SftpConnection, remotePath string, lFile *os.File, offset, chunkSize int64, progressCh chan<- int64) error {
	sftpConn.ReadyWait()

	buf := make([]byte, chunkSize)

	// Read chunk from local file
	n, err := lFile.ReadAt(buf, offset)
	if err != nil && err != io.EOF {
		return fmt.Errorf("error reading local file: %s", err)
	}

	// Open remote file for writing
	rFile, err := sftpConn.Client.OpenFile(remotePath, os.O_WRONLY|os.O_CREATE)
	if err != nil {
		return fmt.Errorf("cannot open remote file for write: %s", err)
	}
	defer rFile.Close()

	// Seek to correct position
	if _, err := rFile.Seek(offset, io.SeekStart); err != nil {
		return fmt.Errorf("cannot seek remote file: %s", err)
	}

	// Write chunk
	totalWritten := 0
	for totalWritten < n {
		written, err := rFile.Write(buf[totalWritten:n])
		if err != nil {
			return fmt.Errorf("error writing remote file: %s", err)
		}
		totalWritten += written
	}

	progressCh <- int64(totalWritten)
	return nil
}

func putFileRecursive(sftpConn *sshc.SftpConnection, remote, local string, maxWorkers int) error {
	sftpConn.ReadyWait()

	remotePath, err := sftpConn.Client.RealPath(remote)
	if err != nil {
		return fmt.Errorf("invalid remote path: %s", remotePath)
	}

	localStat, err := os.Stat(local)
	if err != nil {
		return fmt.Errorf("cannot stat local path: %s", local)
	}
	if !localStat.IsDir() {
		return fmt.Errorf("local path is not a directory: %s", local)
	}

	remoteStat, err := sftpConn.Client.Stat(remotePath)
	if err != nil {
		return fmt.Errorf("cannot stat remote path: %s", remotePath)
	}
	if !remoteStat.IsDir() {
		return fmt.Errorf("local path is not a directory: %s", remotePath)
	}

	dir := filepath.Base(local)
	err = filepath.WalkDir(local, func(localPath string, d fs.DirEntry, err error) error {
		part := strings.TrimPrefix(localPath, local)
		targetPath := filepath.Join(remotePath, dir, part)
		if d.IsDir() {
			err := sftpConn.Client.Mkdir(targetPath)
			if err != nil {
				return fmt.Errorf("cannot create directory %s: %s", remotePath, err)
			}
		} else {
			putFile(sftpConn, targetPath, localPath, maxWorkers)
		}
		return nil
	})
	if err != nil {
		log.Println(err)
	}
	return nil
}

var putCmd = &cobra.Command{
	Use:   "put [user@]host[:port] local [remote]",
	Short: "Puts files from local to remote",
	Long:  `Puts files from local to remote`,
	Example: `
  # uploads a file to the remote server
  $ rospo put myserver:2222 ~/mylocalfolder/myfile.txt /home/myuser/

  # uploads recursively all contents of mylocalfolder to remote current working directory
  $ rospo put myserver:2222 ~/mylocalfolder -r

  # uploads recursively all contents of mylocalfolder to remote target directory
  $ rospo put myserver:2222 ~/mylocalfolder /home/myuser/myremotefolder -r
	`,
	Args: cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		local := args[1]
		remote := ""
		if len(args) > 2 {
			remote = args[2]
		}

		recursive, _ := cmd.Flags().GetBool("recursive")
		maxWorkers, _ := cmd.Flags().GetInt("max-workers")
		sshcConf := cmnflags.GetSshClientConf(cmd, args[0])
		// sshcConf.Quiet = true
		conn := sshc.NewSshConnection(sshcConf)
		go conn.Start()

		sftpConn := sshc.NewSftpConnection(conn)
		go sftpConn.Start()

		if recursive {
			err := putFileRecursive(sftpConn, remote, local, maxWorkers)
			if err != nil {
				putLog.Printf("error while copying file: %s", err)
			}
		} else {
			err := putFile(sftpConn, remote, local, maxWorkers)
			if err != nil {
				putLog.Printf("error while copying file: %s", err)
			}
		}
	},
}
