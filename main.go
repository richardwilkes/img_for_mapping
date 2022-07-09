package main

import (
	"fmt"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/richardwilkes/img_for_mapping/internal/config"
	_ "github.com/richardwilkes/img_for_mapping/internal/webp"
	"github.com/richardwilkes/toolbox/atexit"
	"github.com/richardwilkes/toolbox/cmdline"
	"github.com/richardwilkes/toolbox/errs"
	"github.com/richardwilkes/toolbox/log/jot"
	"github.com/richardwilkes/toolbox/taskqueue"
	"github.com/richardwilkes/toolbox/txt"
	"github.com/richardwilkes/toolbox/xio"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
)

const cwebp = "cwebp"

var (
	sizeRegex = regexp.MustCompile(`(\d+)x(\d+)`)
	ppiRegex  = regexp.MustCompile(`@(\d+) ppi`)
)

func main() {
	cmdline.AppName = "Image Processing for Mapping"
	cmdline.CopyrightYears = "2021-2022"
	cmdline.CopyrightHolder = "Richard A. Wilkes"
	cmdline.AppVersion = "1.1"

	cfg := config.Default()
	cl := cmdline.New(true)
	cfg.AddCmdLineOptions(cl)
	args := cl.Parse(os.Args[1:])

	if _, err := exec.LookPath(cwebp); err != nil {
		jot.Fatal(1, cwebp+" is not installed")
	}
	cfg.Validate()

	queue := taskqueue.New(taskqueue.RecoveryHandler(func(err error) { jot.Error(err) }))
	for _, one := range collectFilesToProcess(args) {
		queue.Submit(createTask(cfg, one))
	}
	queue.Shutdown()
	atexit.Exit(0)
}

func createTask(cfg *config.Config, imgPath string) func() {
	return func() {
		remove, err := processImage(cfg, imgPath)
		if err != nil {
			jot.Error(errs.NewWithCause("unable to process "+imgPath, err))
			return
		}
		if remove {
			if err = os.Remove(imgPath); err != nil {
				jot.Error(errs.NewWithCause("unable to remove "+imgPath, err))
			}
		}
	}
}

func collectFilesToProcess(args []string) []string {
	all := make(map[string]bool)
	extToProcess := make(map[string]bool)
	for _, one := range []string{".png", ".webp", ".jpg", ".jpeg", ".gif", ".tif", ".tiff", ".bmp"} {
		extToProcess[one] = true
	}
	for _, one := range args {
		jot.FatalIfErr(filepath.Walk(one, func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			hidden := strings.HasPrefix(info.Name(), ".")
			if hidden {
				return nil
			}
			if info.IsDir() {
				if hidden {
					return filepath.SkipDir
				}
				return nil
			}
			if extToProcess[strings.ToLower(filepath.Ext(path))] {
				path, err = filepath.Abs(path)
				if err != nil {
					return errs.Wrap(err)
				}
				all[path] = true
			}
			return nil
		}))
	}
	sorted := make([]string, 0, len(all))
	for k := range all {
		sorted = append(sorted, k)
	}
	txt.SortStringsNaturalAscending(sorted)
	return sorted
}

func processImage(cfg *config.Config, imgPath string) (bool, error) {
	img, err := loadImage(imgPath)
	if err != nil {
		return false, err
	}
	b := img.Bounds()
	w := b.Dx()
	h := b.Dy()
	if w%cfg.InputPixelsPerInch != 0 || h%cfg.InputPixelsPerInch != 0 {
		if cfg.KeepGoing {
			return false, nil
		}
		return false, errs.Newf("image does not have a ppi of %d: %s", cfg.InputPixelsPerInch, imgPath)
	}
	newPath := imgPath[:len(imgPath)-len(filepath.Ext(imgPath))]
	if result := sizeRegex.FindStringSubmatchIndex(newPath); len(result) == 6 {
		var inlineW, inlineH int
		if inlineW, err = strconv.Atoi(newPath[result[2]:result[3]]); err != nil {
			inlineW = 0
		}
		if inlineH, err = strconv.Atoi(newPath[result[4]:result[5]]); err != nil {
			inlineH = 0
		}
		if inlineW > 0 && inlineH > 0 && (w != inlineW*cfg.InputPixelsPerInch || h != inlineH*cfg.InputPixelsPerInch) {
			if cfg.KeepGoing {
				return false, nil
			}
			return false, errs.New("image has wrong dimensions: " + imgPath)
		}
		newPath = txt.CollapseSpaces(newPath[:result[0]] + newPath[result[1]:])
	}
	if result := ppiRegex.FindStringSubmatchIndex(newPath); len(result) == 4 {
		var ppi int
		if ppi, err = strconv.Atoi(newPath[result[2]:result[3]]); err != nil {
			ppi = 0
		}
		if ppi != 0 && ppi != cfg.InputPixelsPerInch {
			if cfg.KeepGoing {
				return false, nil
			}
			return false, errs.New("image has wrong pixels-per-inch in name: " + imgPath)
		}
		newPath = txt.CollapseSpaces(newPath[:result[0]] + newPath[result[1]:])
	}
	if w > 16383 {
		rgba, ok := img.(*image.RGBA)
		if !ok {
			b = image.Rect(0, 0, w, h)
			rgba = image.NewRGBA(b)
			draw.Draw(rgba, b, img, b.Min, draw.Src)
		}
		half := image.Rect(0, 0, w/2, h)
		dst := image.NewRGBA(half)
		draw.Draw(dst, half, rgba, b.Min, draw.Src)
		if _, err = writeChunkAsPNG(newPath+" - Left.png", dst, cfg); err != nil {
			return false, err
		}
		draw.Draw(dst, half, rgba, image.Pt(w/2, 0), draw.Src)
		if _, err = writeChunkAsPNG(newPath+" - Right.png", dst, cfg); err != nil {
			return false, err
		}
		return true, nil
	}
	if h > 16383 {
		rgba, ok := img.(*image.RGBA)
		if !ok {
			b = image.Rect(0, 0, w, h)
			rgba = image.NewRGBA(b)
			draw.Draw(rgba, b, img, b.Min, draw.Src)
		}
		half := image.Rect(0, 0, w, h/2)
		dst := image.NewRGBA(half)
		draw.Draw(dst, half, rgba, b.Min, draw.Src)
		if _, err = writeChunkAsPNG(newPath+" - Top.png", dst, cfg); err != nil {
			return false, err
		}
		draw.Draw(dst, half, rgba, image.Pt(0, h/2), draw.Src)
		if _, err = writeChunkAsPNG(newPath+" - Bottom.png", dst, cfg); err != nil {
			return false, err
		}
		return true, nil
	}
	blockWidth := w / cfg.InputPixelsPerInch
	blockHeight := h / cfg.InputPixelsPerInch
	newPath = fmt.Sprintf("%s %dx%d @%d ppi.webp", newPath, blockWidth, blockHeight, cfg.OutputPixelsPerInch)
	if imgPath != newPath || cfg.InputPixelsPerInch != cfg.OutputPixelsPerInch {
		fmt.Println(filepath.Base(imgPath), "->", filepath.Base(newPath))
		needRemove := true
		if imgPath == newPath {
			needRemove = false
			newPath = newPath[:len(newPath)-5] + "_.webp"
		}
		args := make([]string, 0, 16)
		args = append(args, "-preset", "photo", "-q", strconv.Itoa(cfg.Quality), "-m", "6", "-mt", "-af", "-quiet")
		if cfg.InputPixelsPerInch != cfg.OutputPixelsPerInch {
			args = append(args, "-resize", strconv.Itoa(blockWidth*cfg.OutputPixelsPerInch),
				strconv.Itoa(blockHeight*cfg.OutputPixelsPerInch))
		}
		args = append(args, "-o", newPath, imgPath)
		cmd := exec.Command(cwebp, args...)
		var out []byte
		out, err = cmd.CombinedOutput()
		if err != nil {
			return false, errs.NewWithCause("unable to process "+imgPath+"\n\n"+string(out), err)
		}
		str := strings.TrimSpace(string(out))
		if str != "" && str != "libpng warning: iCCP: known incorrect sRGB profile" {
			fmt.Println(str)
		}
		if !needRemove {
			jot.FatalIfErr(os.Remove(imgPath))
			jot.FatalIfErr(os.Rename(newPath, newPath[:len(newPath)-6]+".webp"))
		}
		return needRemove, nil
	}
	return false, nil
}

func loadImage(imgPath string) (image.Image, error) {
	f, err := os.Open(imgPath)
	if err != nil {
		return nil, errs.NewWithCause("unable to open image: "+imgPath, err)
	}
	defer xio.CloseIgnoringErrors(f)
	var img image.Image
	if img, _, err = image.Decode(f); err != nil {
		return nil, errs.NewWithCause("unable to decode image: "+imgPath, err)
	}
	return img, nil
}

func writeChunkAsPNG(filePath string, img image.Image, cfg *config.Config) (bool, error) {
	f, err := os.Create(filePath)
	if err != nil {
		return false, errs.NewWithCause("unable to create "+filePath, err)
	}
	if err = png.Encode(f, img); err != nil {
		return false, errs.NewWithCause("unable to encode "+filePath, err)
	}
	if err = f.Close(); err != nil {
		return false, errs.NewWithCause("unable to encode "+filePath, err)
	}
	defer func() {
		jot.FatalIfErr(os.Remove(filePath))
	}()
	return processImage(cfg, filePath)
}
