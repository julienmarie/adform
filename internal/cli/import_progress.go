package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"adform/internal/importer"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

type importProgressRenderer struct {
	w       io.Writer
	inPlace bool
	p       *mpb.Progress

	bars     map[string]*mpb.Bar
	lastPct  map[string]int
	lastCur  map[string]int
	lastLine map[string]string
	mu       sync.Mutex
	once     sync.Once
}

func newImportProgressRenderer(w io.Writer) *importProgressRenderer {
	r := &importProgressRenderer{
		w:        w,
		bars:     map[string]*mpb.Bar{},
		lastPct:  map[string]int{},
		lastCur:  map[string]int{},
		lastLine: map[string]string{},
	}
	if f, ok := w.(*os.File); ok {
		if fi, err := f.Stat(); err == nil && (fi.Mode()&os.ModeCharDevice) != 0 {
			r.inPlace = true
		}
	}
	if r.inPlace {
		r.p = mpb.New(
			mpb.WithOutput(w),
			mpb.WithWidth(24),
			mpb.WithRefreshRate(120*time.Millisecond),
		)
	}
	return r
}

func (r *importProgressRenderer) Close() {
	r.once.Do(func() {
		if r.p != nil {
			r.p.Wait()
		}
	})
}

func (r *importProgressRenderer) Handle(ev importer.ProgressEvent) {
	if ev.Stage == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if ev.Total <= 0 {
		r.renderLineOnly(ev)
		return
	}
	if ev.Current < 0 {
		ev.Current = 0
	}
	if ev.Current > ev.Total {
		ev.Current = ev.Total
	}
	pct := int(float64(ev.Current) * 100 / float64(ev.Total))

	if !ev.Done {
		lastPct := r.lastPct[ev.Stage]
		lastCur := r.lastCur[ev.Stage]
		if ev.Current != ev.Total && pct < 100 && (pct-lastPct < 2) && (ev.Current-lastCur < 20) {
			return
		}
	}
	r.lastPct[ev.Stage] = pct
	r.lastCur[ev.Stage] = ev.Current

	if r.inPlace {
		r.renderBar(ev)
		return
	}
	r.renderLineOnly(ev)
}

func (r *importProgressRenderer) renderBar(ev importer.ProgressEvent) {
	bar := r.bars[ev.Stage]
	if bar == nil {
		bar = r.p.New(
			int64(ev.Total),
			mpb.BarStyle().Lbound("[").Filler("=").Tip(">").Padding("-").Rbound("]"),
			mpb.PrependDecorators(
				decor.Name(fmt.Sprintf("[import] %-14s ", stageLabel(ev.Stage)), decor.WCSyncWidthR),
			),
			mpb.AppendDecorators(
				decor.Percentage(decor.WCSyncWidthR),
				decor.Name(" "),
				decor.CountersNoUnit("(%d/%d)", decor.WCSyncWidth),
			),
		)
		r.bars[ev.Stage] = bar
	} else {
		bar.SetTotal(int64(ev.Total), false)
	}
	bar.SetCurrent(int64(ev.Current))
	if ev.Done || ev.Current == ev.Total {
		bar.SetTotal(int64(ev.Total), true)
	}
}

func (r *importProgressRenderer) renderLineOnly(ev importer.ProgressEvent) {
	if ev.Total <= 0 {
		line := ""
		if ev.Message != "" {
			line = fmt.Sprintf("[import] %-14s %s", stageLabel(ev.Stage), ev.Message)
		} else if ev.Done {
			line = fmt.Sprintf("[import] %-14s done", stageLabel(ev.Stage))
		}
		if line != "" && r.lastLine[ev.Stage] != line {
			r.lastLine[ev.Stage] = line
			fmt.Fprintln(r.w, line)
		}
		return
	}

	pct := int(float64(ev.Current) * 100 / float64(ev.Total))
	line := fmt.Sprintf("[import] %-14s %s %3d%% (%d/%d)", stageLabel(ev.Stage), bar(pct, 24), pct, ev.Current, ev.Total)
	if ev.Message != "" {
		line += " " + ev.Message
	}
	if r.lastLine[ev.Stage] == line {
		return
	}
	r.lastLine[ev.Stage] = line
	fmt.Fprintln(r.w, line)
}

func stageLabel(stage string) string {
	switch stage {
	case "campaigns":
		return "campaigns"
	case "adsets":
		return "adsets"
	case "ads":
		return "ads"
	case "creatives":
		return "creatives"
	case "audiences":
		return "audiences"
	case "assets-image":
		return "assets:image"
	case "assets-video":
		return "assets:video"
	case "catalogs":
		return "catalogs"
	case "details":
		return "details"
	case "scaffold":
		return "scaffold"
	case "write-files":
		return "write-files"
	default:
		return stage
	}
}

func bar(pct, width int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	fill := pct * width / 100
	if fill > width {
		fill = width
	}
	if fill < 0 {
		fill = 0
	}
	return "[" + strings.Repeat("=", fill) + strings.Repeat("-", width-fill) + "]"
}
