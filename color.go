package kubelogtail

import (
	"fmt"
	"sync"

	"github.com/fatih/color"
	"github.com/pkg/errors"
)

// color mode
const (
	logColorLine = iota // whole line - the default
	logColorPod         // just the pod
	logColorOff         // no color
)

type colorFunc func(string, string)

type logColorPrint struct {
	sync.Mutex
	colorFuncs   []colorFunc
	currentColor int //index of current color
	mode         int
}

func newLogColorPrint(mode string) (*logColorPrint, error) {
	m := logColorLine
	switch mode {
	case "off":
		m = logColorOff
	case "pod":
		m = logColorPod
	case "line", "":
		m = logColorLine
	default:
		return nil, errors.Errorf("unknown color print mode: \"%s\"", mode)
	}

	l := logColorPrint{
		mode: m,
	}

	l.generateColors()
	return &l, nil
}

type fmtPrinter func(format string, a ...interface{}) string

func (l *logColorPrint) generateColors() {
	l.Lock()
	defer l.Unlock()

	if l.mode == logColorOff {
		f := func(label, line string) {
			fmt.Println(label, line)
		}
		l.colorFuncs = []colorFunc{f}
		return
	}
	colors := []fmtPrinter{color.BlueString, color.CyanString, color.YellowString, color.RedString, color.MagentaString}

	for i := range colors {
		f := colors[i]
		var cf colorFunc

		if l.mode == logColorPod {
			cf = func(label, line string) {
				fmt.Println(f(label), line)
			}
		} else {
			cf = func(label, line string) {
				f("%s %s\n", label, line)
			}
		}
		l.colorFuncs = append(l.colorFuncs, cf)
	}
}

func (l *logColorPrint) getColor() colorFunc {
	l.Lock()
	defer l.Unlock()
	l.currentColor++
	if l.currentColor >= len(l.colorFuncs) {
		l.currentColor = 0
	}
	return l.colorFuncs[l.currentColor]
}
