// Swap MKV/M4A containers to MP4/MP3.
// Usage: media-swapper --src=/path/to/videos --bin=/path/to/ffmpeg
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/ziadoz/media-swapper/pkg/fs"
	"github.com/ziadoz/media-swapper/pkg/pathflag"
	"github.com/ziadoz/media-swapper/pkg/swap"
)

var bin pathflag.Path
var src pathflag.Path

type result struct {
	cmd *swap.Cmd
	err error
}

func init() {
	flag.Var(&bin, "bin", "The location of the ffmpeg or avconv binary")
	flag.Var(&src, "src", "The source directory of mkv/m4a files or an individual mkv/m4a file to swap to mp4/mp3")
	flag.Parse()
}

func main() {
	if bin.Path == "" {
		fmt.Fprintln(os.Stderr, "The -bin flag must be specified")
		os.Exit(1)
	}

	if src.Path == "" {
		fmt.Fprintln(os.Stderr, "The -src flag must be specified")
		os.Exit(1)
	}

	files, err := fs.GetSwappableFiles(src)
	if err != nil || len(files) == 0 {
		fmt.Fprintf(os.Stderr, "Could not find mkv/m4a files: %s\n", err)
		os.Exit(1)
	}

	workers := len(files) / 2
	if workers == 0 {
		workers = 5
	}

	fmt.Printf("Swapping %d videos: \n", len(files))

	in := make(chan *swap.Cmd)
	out := make(chan *result)
	done := make(chan struct{})
	wg := sync.WaitGroup{}

	go results(out, done)
	go pool(&wg, workers, in, out)
	go queue(files, in)

	<-done
}

func queue(files []string, in chan *swap.Cmd) {
	for _, file := range files {
		if fs.IsSwappableVideo(file) {
			in <- swap.Mp4Command(bin.Path, file)
		} else if fs.IsSwappableAudio(file) {
			in <- swap.Mp3Command(bin.Path, file)
		}
	}

	close(in)
}

func pool(wg *sync.WaitGroup, workers int, in chan *swap.Cmd, out chan *result) {
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go worker(wg, in, out)
	}

	wg.Wait()
	close(out)
}

func worker(wg *sync.WaitGroup, in chan *swap.Cmd, out chan *result) {
	for cmd := range in {
		var cmdout, cmderr bytes.Buffer
		cmd.Stdout = &cmdout
		cmd.Stderr = &cmderr

		var reserr error
		if err := cmd.Run(); err != nil {
			reserr = err
			if strings.Contains(cmderr.String(), "already exists. Overwrite ? [y/N]") {
				reserr = fmt.Errorf("mp4 file already exists")
			}
		}

		out <- &result{
			cmd: cmd,
			err: reserr,
		}
	}

	wg.Done()
}

func results(out chan *result, done chan struct{}) {
	for result := range out {
		if result.err != nil {
			fmt.Printf(" - Failed: %s: %s\n", result.cmd.Input, result.err)
		} else {
			fmt.Printf(" - Swapped: %s\n", result.cmd.Input)
		}
	}

	done <- struct{}{}
}