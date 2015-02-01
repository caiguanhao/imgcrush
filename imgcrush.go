package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	TYPE_JPG = 0
	TYPE_PNG = 1
	TYPE_GIF = 2
)

type Image struct {
	Type          int64
	BeforePath    string
	BeforePathRel string
	BeforeSize    int64
	AfterPath     string
	AfterSize     int64
	Err           error
	ErrMsg        string
}

func (image Image) command() *exec.Cmd {
	var cmd *exec.Cmd
	switch image.Type {
	case TYPE_GIF:
		cmd = exec.Command("gifsicle", "-O3", "--careful",
			"--no-comments", "--no-names", "--no-warnings",
			"--same-delay", "--same-loopcount",
			"--output", image.AfterPath, image.BeforePath)
	case TYPE_PNG:
		cmd = exec.Command("pngcrush", "-q", image.BeforePath, image.AfterPath)
	default:
		cmd = exec.Command("mozjpeg", "-copy", "none", "-outfile",
			image.AfterPath, image.BeforePath)
	}
	return cmd
}

func (image Image) mkdir() error {
	dir := path.Dir(image.AfterPath)
	return os.MkdirAll(dir, 0755)
}

func (image *Image) crush() {
	var err error
	var bytes []byte
	cmd := image.command()
	stderr, err := cmd.StderrPipe()

	err = cmd.Start()
	if err != nil {
		image.Err = err
		return
	}

	bytes, err = ioutil.ReadAll(stderr)
	if err != nil {
		image.Err = err
		return
	}
	image.ErrMsg = string(bytes)

	err = cmd.Wait()
	if err != nil {
		image.Err = err
		return
	}
}

func (image *Image) calculateAfterSize() {
	file, err := os.Open(image.AfterPath)
	if err != nil {
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return
	}
	image.AfterSize = stat.Size()
}

func imageTypeByName(name string) int64 {
	name = strings.ToLower(name)

	if strings.HasSuffix(name, ".gif") {
		return TYPE_GIF
	}

	if strings.HasSuffix(name, ".jpg") || strings.HasSuffix(name, ".jpeg") {
		return TYPE_JPG
	}

	if strings.HasSuffix(name, ".png") {
		return TYPE_PNG
	}

	return -1
}

func findImages(done <-chan struct{}, inputs *[]string, output *string) (<-chan Image, <-chan error) {
	length := len(*inputs)
	images := make(chan Image)
	errs := make(chan error, length)
	var wg sync.WaitGroup
	wg.Add(length)
	for _, input := range *inputs {
		go func(input string) {
			defer wg.Done()
			errs <- filepath.Walk(input, func(beforePath string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if !info.Mode().IsRegular() {
					return nil
				}

				imageType := imageTypeByName(info.Name())
				if imageType < 0 {
					return nil
				}

				beforePath, ferr := filepath.Abs(beforePath)
				if ferr != nil {
					return ferr
				}

				if strings.HasPrefix(beforePath, *output) {
					return nil
				}

				beforePathRel, ferr := filepath.Rel(input, beforePath)
				if ferr != nil {
					return ferr
				}

				if beforePathRel == "." {
					beforePathRel = filepath.Base(beforePath)
				}

				afterPath := path.Join(*output, beforePathRel)
				image := Image{
					Type:          imageType,
					BeforePathRel: beforePathRel,
					BeforePath:    beforePath,
					BeforeSize:    info.Size(),
					AfterPath:     afterPath,
				}

				select {
				case images <- image:
				case <-done:
					return errors.New("walk cancelled")
				}
				return nil
			})
		}(input)
	}
	go func() {
		wg.Wait()
		close(images)
	}()
	return images, errs
}

func crush(done <-chan struct{}, images <-chan Image, c chan<- Image) {
	for image := range images {
		image.mkdir()
		image.crush()
		image.calculateAfterSize()
		select {
		case c <- image:
		case <-done:
			return
		}
	}
}

func crushAll(concurrency int, inputs *[]string, output *string) error {
	done := make(chan struct{})
	defer close(done)

	images, errs := findImages(done, inputs, output)

	c := make(chan Image)
	var wg sync.WaitGroup
	wg.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		go func() {
			crush(done, images, c)
			wg.Done()
		}()
	}

	go func() {
		wg.Wait()
		close(c)
	}()

	for image := range c {
		if image.Err != nil {
			fmt.Fprintf(os.Stderr, "file: %s error:\n", image.BeforePathRel)
			fmt.Fprint(os.Stderr, image.ErrMsg)
			continue
		}
		fmt.Printf("file: %s before: %d after: %d reduced: %.2f%%\n",
			image.BeforePathRel, image.BeforeSize, image.AfterSize,
			float64(image.BeforeSize-image.AfterSize)/float64(image.BeforeSize)*100)
	}

	if err := <-errs; err != nil {
		return err
	}

	return nil
}

func main() {
	var concurrency int
	var input []string
	var output string

	flag.IntVar(&concurrency, "c", 2, "")
	flag.StringVar(&output, "o", "done", "")
	flag.Usage = func() {
		fmt.Println("Recursively, losslessly and quickly compress JPG, PNG, GIF files.")
		fmt.Println()
		fmt.Printf("Usage: %s [-c 2] [-o done] [DIRECTORY] ...\n", path.Base(os.Args[0]))
		fmt.Println("  -c <num>  concurrency from 1 to 8, defaults to 2")
		fmt.Println("  -o <dir>  output directory, defaults to done")
		fmt.Println()
		fmt.Println("If no input directory provided, it will use current directory.")
		fmt.Println("Images in output directory will not be used as input images.")
	}
	flag.Parse()

	input = flag.Args()
	if len(input) == 0 {
		input = append(input, ".")
	}

	var err error
	for i := range input {
		input[i], err = filepath.Abs(input[i])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}
	output, err = filepath.Abs(output)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}

	timeStart := time.Now()

	if concurrency > 8 || concurrency < 1 {
		concurrency = 2
	}

	err = crushAll(concurrency, &input, &output)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}

	fmt.Printf("time used %.3f secs\n", time.Since(timeStart).Seconds())
}
