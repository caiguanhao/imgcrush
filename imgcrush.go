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
	"syscall"
	"time"
	"unsafe"
)

const (
	TYPE_JPG = 0
	TYPE_PNG = 1
	TYPE_GIF = 2

	FILE_PADDING  = 1
	SIZE_WIDTH    = 10
	PERCENT_WIDTH = 9
	SECONDS_WIDTH = 10
)

var (
	OLD_STYLE_PRINT bool
	TERM_WIDTH      int
	FILE_WIDTH      int
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
	TimeUsed      float64
}

func (image *Image) from(from Image) {
	image.BeforePathRel = from.BeforePathRel
	image.BeforeSize = from.BeforeSize
	image.AfterSize = from.AfterSize
	image.TimeUsed = from.TimeUsed
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
	timeStart := time.Now()
	defer func() {
		image.TimeUsed = time.Since(timeStart).Seconds()
	}()
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

func (image Image) print() {
	reduced := float64(image.BeforeSize-image.AfterSize) / float64(image.BeforeSize) * 100

	if OLD_STYLE_PRINT {
		fmt.Printf("file: %s before: %d after: %d reduced: %.2f%% time used: %.3f secs\n",
			image.BeforePathRel, image.BeforeSize, image.AfterSize, reduced, image.TimeUsed)
		return
	}

	fileR := []rune(image.BeforePathRel)
	var file string
	if len(fileR) > FILE_WIDTH {
		file = string(fileR[0:FILE_WIDTH])
	} else {
		file = string(fileR)
	}

	fmt.Printf("%-*s", FILE_WIDTH, file)
	fmt.Printf("%*s", FILE_PADDING+2, " ")
	fmt.Printf("%*d", SIZE_WIDTH, image.BeforeSize)
	fmt.Printf("%*d", SIZE_WIDTH, image.AfterSize)
	fmt.Printf("%*.2f%%", PERCENT_WIDTH, reduced)
	fmt.Printf("%*.3fs", SECONDS_WIDTH, image.TimeUsed)
	fmt.Println()
}

func printHeader() {
	if OLD_STYLE_PRINT {
		return
	}

	fmt.Printf("%-*s", FILE_WIDTH, "FILE")
	fmt.Printf("%*s", FILE_PADDING+2, " ")
	fmt.Printf("%*s", SIZE_WIDTH, "BEFORE")
	fmt.Printf("%*s", SIZE_WIDTH, "AFTER")
	fmt.Printf("%*s", PERCENT_WIDTH+1, "REDUCED")
	fmt.Printf("%*s", SECONDS_WIDTH+1, "TIME")
	fmt.Println()
}

func printTotal(maxTimeImage Image, timeStart time.Time, beforeTotal, afterTotal int64) {
	maxTimeImageReduced := float64(maxTimeImage.BeforeSize-maxTimeImage.AfterSize) / float64(maxTimeImage.BeforeSize) * 100
	reduced := float64(beforeTotal-afterTotal) / float64(beforeTotal) * 100

	if OLD_STYLE_PRINT {
		fmt.Println("-----")
		fmt.Printf("total: %s (before: %d after: %d reduced: %.2f%%) took the longest time (%.3f secs) to complete\n",
			maxTimeImage.BeforePathRel, maxTimeImage.BeforeSize, maxTimeImage.AfterSize,
			maxTimeImageReduced, maxTimeImage.TimeUsed)
		fmt.Printf("total: before: %d after: %d reduced: %d (%.2f%%) time used: %.3f secs\n",
			beforeTotal, afterTotal, beforeTotal-afterTotal,
			reduced, time.Since(timeStart).Seconds())
		return
	}

	fmt.Printf("%-*s", FILE_WIDTH, fmt.Sprintf("(longest time) %s", maxTimeImage.BeforePathRel))
	fmt.Printf("%*s", FILE_PADDING+2, " ")
	fmt.Printf("%*d", SIZE_WIDTH, maxTimeImage.BeforeSize)
	fmt.Printf("%*d", SIZE_WIDTH, maxTimeImage.AfterSize)
	fmt.Printf("%*.2f%%", PERCENT_WIDTH, maxTimeImageReduced)
	fmt.Printf("%*.3fs", SECONDS_WIDTH, maxTimeImage.TimeUsed)
	fmt.Println()

	fmt.Printf("%-*s", FILE_WIDTH, "(total)")
	fmt.Printf("%*s", FILE_PADDING+2, " ")
	fmt.Printf("%*d", SIZE_WIDTH, beforeTotal)
	fmt.Printf("%*d", SIZE_WIDTH, afterTotal)
	fmt.Printf("%*.2f%%", PERCENT_WIDTH, reduced)
	fmt.Printf("%*.3fs", SECONDS_WIDTH, time.Since(timeStart).Seconds())
	fmt.Println()
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

	imagesChannel := make(chan Image)
	var wg sync.WaitGroup
	wg.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		go func() {
			crush(done, images, imagesChannel)
			wg.Done()
		}()
	}

	go func() {
		wg.Wait()
		close(imagesChannel)
	}()

	printHeader()

	var beforeTotal, afterTotal int64
	var maxTimeImage Image
	timeStart := time.Now()
	for image := range imagesChannel {
		if image.Err != nil {
			fmt.Fprintf(os.Stderr, "file: %s error:\n", image.BeforePathRel)
			fmt.Fprint(os.Stderr, image.ErrMsg)
			continue
		}
		beforeTotal += image.BeforeSize
		afterTotal += image.AfterSize
		image.print()
		if image.TimeUsed > maxTimeImage.TimeUsed {
			maxTimeImage.from(image)
		}
	}
	if beforeTotal > 0 && afterTotal > 0 {
		printTotal(maxTimeImage, timeStart, beforeTotal, afterTotal)
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
	flag.BoolVar(&OLD_STYLE_PRINT, "old-style", false, "")
	flag.Usage = func() {
		fmt.Println("Recursively, losslessly and quickly compress JPG, PNG, GIF images.")
		fmt.Println()
		fmt.Printf("Usage: %s [-c 2] [-o done] [DIRECTORY] ...\n", path.Base(os.Args[0]))
		fmt.Println("  -c <num>     concurrency from 1 to 8, defaults to 2")
		fmt.Println("  -o <dir>     output directory, defaults to done")
		fmt.Println("  --old-style  print results using old style")
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

	TERM_WIDTH, _, err = getTerminalSize()
	FILE_WIDTH = TERM_WIDTH - (SIZE_WIDTH+1)*2 - (PERCENT_WIDTH + 1) - (SECONDS_WIDTH + 1) - FILE_PADDING

	if concurrency > 8 || concurrency < 1 {
		concurrency = 2
	}

	err = crushAll(concurrency, &input, &output)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

func getTerminalSize() (width, height int, err error) {
	var dimensions [4]uint16
	if _, _, err := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(syscall.Stdin),
		uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&dimensions)),
		0, 0, 0); err != 0 {
		return -1, -1, err
	}
	return int(dimensions[1]), int(dimensions[0]), nil
}
