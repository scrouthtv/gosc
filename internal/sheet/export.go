package sheet

import (
	"bufio"
	"fmt"
	"os"
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

func (s *Sheet) exportCell(c *Cell, a Address) string {
	if c == nil {
		return "          "
	}
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
