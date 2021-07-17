package sheet

import (
	"bufio"
	"errors"
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

	for x := 0; x <= w; x++ {
		for y := 0; y <= h; y++ {
			a := NewAddress(x, y)
			c, err := s.GetCell(a)
			if err != nil {
				if errors.Is(err, &ErrCellNotFound{}) {
					continue
				}

				return err
			}

			if c == nil {
				continue
			}

			_, err = out.WriteString(s.exportCell(c, a))
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
	if len(str) >= width {
		return str[:width]
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

func (s *Sheet) exportCell(c *Cell, a Address) string {
	w := s.getColumnWidth(a.ColumnHeader())
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
	var buf strings.Builder
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
