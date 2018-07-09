package servicetest

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"

	service "github.com/shabbyrobe/go-service"
)

func fuzzOutput(method string, name string, stats *FuzzStats, w io.Writer) {
	var err error
	stats = stats.Clone()
	switch method {
	case "json":
		err = fuzzOutputJSON(name, stats, w)
	default:
		err = fuzzOutputCLI(name, stats, w)
	}
	if err != nil {
		panic(err)
	}
}

func fuzzOutputJSON(name string, stats *FuzzStats, w io.Writer) error {
	e := json.NewEncoder(w)
	e.SetIndent("", "  ")
	return e.Encode(stats)
}

func fuzzOutputCLI(name string, stats *FuzzStats, w io.Writer) error {
	var (
		headingw = 18
		rowheadw = 18
		okerrw   = 8

		heading    = func(v interface{}) string { return colorwr(lightBlue, headingw, ' ', v) }
		rowhead    = func(v interface{}) string { return colorwr(lightBlue, rowheadw, ' ', v) }
		subheading = func(v interface{}) string { return color(darkGray, v) }
		value      = func(v interface{}) string { return color(white, v) }
		colhead    = func(v interface{}) string { return colorwr(lightCyan, okerrw, ' ', v) }
		pctcol     = func(v interface{}) string { return colorwr(lightGray, okerrw, ' ', v) }

		okcol = func(v interface{}) string {
			col := lightGreen
			if v == 0 {
				col = lightGray
			}
			return colorwr(col, okerrw, ' ', v)
		}

		errcol = func(v interface{}) string {
			col := lightRed
			if v == 0 {
				col = lightGray
			}
			return colorwr(col, okerrw, ' ', v)
		}
	)

	fmt.Fprintf(w, "%s  %s\n", heading("seed"), value(stats.Seed))
	fmt.Fprintf(w, "%s  %s (%s)\n",
		heading("duration"), color(lightCyan, stats.Duration),
		color(lightCyan, stats.Tick),
	)

	fmt.Fprintf(w, "%s  %s/%s ", heading("starts/ends"), value(stats.Starts()), value(stats.Ends()))

	diff := stats.Ends() - stats.Starts()
	if diff != 0 {
		fmt.Fprintf(w, "%s", color(red, diff))
	}
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "%s  ", heading("states"))
	for _, state := range service.States {
		count := stats.StateCheckResults[state]
		fmt.Fprintf(w, "%s:%s ", subheading(state.String()), value(count))
	}
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "%s  %s:%s %s:%s %s:%s\n", heading("runners"),
		subheading("current"), value(stats.RunnersCurrent),
		subheading("halted"), value(stats.RunnersHalted),
		subheading("started"), value(stats.RunnersStarted))

	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "%s %s %s %s\n", rowhead(""),
		colhead("svc ok"), colhead("svc err"), colhead("svc pct"))

	counterRow := func(head string, svc *ErrorCounter) {
		fmt.Fprintf(w, "%s %s %s %s\n", rowhead(head),
			okcol(svc.Succeeded()),
			errcol(svc.Failed()),
			pctcol(math.Round(svc.Percent())))
	}

	counterRow("start", stats.Service.ServiceStart)
	counterRow("halt", stats.Service.ServiceHalt)
	counterRow("restart", stats.Service.ServiceRestart)

	fmt.Fprintf(w, "\n")

	errs := stats.Errors()
	if len(errs) > 0 {
		sort.Slice(errs, func(i, j int) bool { return errs[i].Name < errs[j].Name })
		nameLen := 0
		numLen := 0
		for _, e := range errs {
			nlen := len(e.Name)
			if nlen > nameLen {
				nameLen = nlen
			}
			nums := fmt.Sprintf("x%d", e.Count)
			nlen = len(nums)
			if nlen > numLen {
				numLen = nlen
			}
		}

		fmt.Fprintf(w, "%d error(s):\n", len(errs))

		for _, e := range errs {
			fmt.Fprintf(w, "%s %s %s\n",
				colorwr(lightRed, nameLen, ' ', e.Name),
				colorwl(lightCyan, numLen, ' ', fmt.Sprintf("x%d", e.Count)),
				e.Err)
		}
		fmt.Fprintf(w, "\n")
	}

	return nil
}

const (
	black        = 30
	red          = 31
	green        = 32
	yellow       = 33
	blue         = 34
	magenta      = 35
	cyan         = 36
	lightGray    = 37
	darkGray     = 90
	lightRed     = 91
	lightGreen   = 92
	lightYellow  = 93
	lightBlue    = 94
	lightMagenta = 95
	lightCyan    = 96
	white        = 97
)

func color(col int, v interface{}) string {
	return fmt.Sprintf("\x1b[%dm%v\x1b[0m", col, v)
}

func colorwl(col int, w int, c byte, v interface{}) string {
	vs := fmt.Sprintf("%v", v)
	vl := len(vs)
	vs += strings.Repeat(string(c), w-vl)
	return fmt.Sprintf("\x1b[%dm%v\x1b[0m", col, vs)
}

func colorwr(col int, w int, c byte, v interface{}) string {
	vs := fmt.Sprintf("%v", v)
	cs := string(c)
	for i := len(vs); i < w; i++ {
		vs = cs + vs
	}
	return fmt.Sprintf("\x1b[%dm%v\x1b[0m", col, vs)
}
