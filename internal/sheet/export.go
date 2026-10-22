package sheet

import (
	"bufio"
	"math"
	"os"
	"regexp"
	"strings"

	"github.com/pedroalbanese/sc/internal/sheet/align"
)

// stripTags removes formatting tags from a string, returning only
// the visible text. Adjust the regex to match the tags you use.
func stripTags(s string) string {
	re := regexp.MustCompile(`\[[^\]]*\]`)
	return re.ReplaceAllString(s, "")
}

// longestWordLen returns the length of the longest word in a string,
// ignoring formatting tags.
func longestWordLen(s string) int {
	visible := stripTags(s)
	words := strings.Fields(visible)
	max := 0
	for _, w := range words {
		if len(w) > max {
			max = len(w)
		}
	}
	return max
}

// exportColumnWidths computes, for each column, the width needed to
// fit the longest visible word in that column (ignoring tags).
// It returns a map from column header to width.
func (s *Sheet) exportColumnWidths() map[string]int {
	widths := make(map[string]int)
	for addr, cell := range s.data {
		col := addr.ColumnHeader()
		disp := cell.getDisplay(s, addr)
		w := longestWordLen(disp) + 1 // one space of padding
		if w < initial_COLUMN_WIDTH {
			w = initial_COLUMN_WIDTH
		}
		if w > widths[col] {
			widths[col] = w
		}
	}
	// Ensure the column header itself fits.
	for col := range widths {
		headerLen := len(col) + 1
		if headerLen > widths[col] {
			widths[col] = headerLen
		}
	}
	return widths
}

func (s *Sheet) Export(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}

	defer f.Close()
	out := bufio.NewWriter(f)
	w, h := s.Size()

	// Compute export-specific column widths.
	widths := s.exportColumnWidths()

	for x := 0; x <= w; x++ {
		for y := 0; y <= h; y++ {
			a := NewAddress(x, y)
			c, err := s.GetCell(a)
			if err != nil {
				col := a.ColumnHeader()
				cw := widths[col]
				if cw == 0 {
					cw = initial_COLUMN_WIDTH
				}
				out.WriteString(spaces(cw))
				continue
			}

			if c == nil {
				continue
			}

			_, err = out.WriteString(s.exportCellWithWidth(c, a, widths[a.ColumnHeader()]))
			if err != nil {
				return err
			}
		}
		_, err = out.WriteRune('\n')
		if err != nil {
			return err
		}
	}

	return out.Flush()
}

func alignText(str string, a align.Align, width int) string {
	// Do not truncate: if the string is longer than the column width,
	// return it as is.
	if len(str) >= width {
		return str
	}
	extra := width - len(str)

	switch a {
	case align.AlignLeft:
		return str + spaces(extra)
	case align.AlignRight:
		return spaces(extra) + str
	case align.AlignCenter:
		fallthrough
	default:
		l := int(math.Ceil(float64(extra) / 2.0))
		r := int(math.Floor(float64(extra) / 2.0))
		return spaces(l) + str + spaces(r)
	}
}

func (s *Sheet) exportCellWithWidth(c *Cell, a Address, w int) string {
	if c == nil {
		return spaces(w)
	} else if c.stringType {
		return alignText(c.value, c.alignment, w)
	} else {
		t := c.getDisplayValue(s, a)
		return alignText(t, align.AlignRight, w)
	}
}

func spaces(n int) string {
	return strings.Repeat(" ", n)
}

func (s *Sheet) Size() (width int, height int) {
	if s.loading {
		return 0, 0
	}

	w, h := 0, 0
	for a := range s.data {
		if a.Row() > w {
			w = a.Row()
		}

		if a.Column() > h {
			h = a.Column()
		}
	}

	return w, h
}
