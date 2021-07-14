package sheet

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"strings"
	"github.com/scrouthtv/gosc/internal/sheet/align"
)

func (s *Sheet) Export(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}

	defer f.Close()
	out := bufio.NewWriter(f)
	w, h := s.Size()

	out.WriteString(fmt.Sprintf("size: %d %d\n", w, h))

	for x := 0; x <= w; x++ {
		for y := 0; y <= h; y++ {
			a := NewAddress(x, y)
			c, err := s.GetCell(a)
			if err != nil {
				return err
			}

			if c == nil {
				continue
			}

			_, err = out.WriteString(c.getDisplay(s, a))
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

func alignText(str string, a Align, w width) string {
	if len(t) >= w {
		return text[:w]
	}
	extra := w - len(c.value)
	
	switch c.alignment {
	case align.AlignLeft:
		return text + spaces(extra)
	case align.AlignRight:
		return spaces(extra) + text
	case align.AlignCenter:
		fallthrough
	default:
		l := math.Ceil(extra / 2.0)
		r := math.Floor(extra / 2.0)
		return spaces(l) + text + spaces(r)
	}
}

func (s *Sheet) exportCell(c *Cell, a Address) string {
	w := getColumnWidth(a.ColumnHeader())
	if c == nil {
		return spaces(w)
	} else i c.stringType {
		return alignText(c.value, c.alignment, w)
	} else {
		t := c.getDisplayValue(s, a)
		return alignText(t, align.AlignRight, w)
	}
}

func spaces(n int) string {
	var buf strings.Buffer
	for i := 0; i < n; i++ {
		buf.WriteRune(' ')
	}
	return buf.String()
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
