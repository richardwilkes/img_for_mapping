package config

import (
	"log/slog"

	"github.com/richardwilkes/toolbox/atexit"
	"github.com/richardwilkes/toolbox/cmdline"
)

type Config struct {
	InputPixelsPerInch  int
	OutputPixelsPerInch int
	Quality             int
	KeepGoing           bool
}

func Default() *Config {
	return &Config{
		InputPixelsPerInch:  140,
		OutputPixelsPerInch: 0,
		Quality:             75,
	}
}

func (c *Config) AddCmdLineOptions(cl *cmdline.CmdLine) {
	cl.NewGeneralOption(&c.InputPixelsPerInch).SetSingle('p').SetName("ppi").
		SetUsage("The expected pixels-per-inch of the image files")
	cl.NewGeneralOption(&c.OutputPixelsPerInch).SetSingle('r').SetName("resize-ppi").
		SetUsage("The output pixels-per-inch of the image files. If unset, defaults to the input pixels-per-inch")
	cl.NewGeneralOption(&c.Quality).SetSingle('q').SetName("quality").SetUsage("The desired quality")
	cl.NewGeneralOption(&c.KeepGoing).SetSingle('k').SetName("keep-going").SetUsage("Keep going, skipping over image files that are invalid due to ppi or size mis-matches")
}

func (c *Config) Validate() {
	if c.InputPixelsPerInch < 50 || c.InputPixelsPerInch > 600 {
		slog.Error("input pixels per inch must be in the range 50 to 600")
		atexit.Exit(1)
	}

	if c.OutputPixelsPerInch == 0 {
		c.OutputPixelsPerInch = c.InputPixelsPerInch
	} else if c.OutputPixelsPerInch < 50 || c.OutputPixelsPerInch > 600 {
		slog.Error("output pixels per inch must be either unset or in the range 50 to 600")
		atexit.Exit(1)
	}

	if c.Quality < 0 || c.Quality > 100 {
		slog.Error("quality must be in the range 0 to 100")
		atexit.Exit(1)
	}
}
