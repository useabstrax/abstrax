package daemon

import (
	"bytes"
	"io"
	"os"
)

// tailLines returns the last n lines of path without reading the whole file
// when the file is larger than the trailing window.
func tailLines(path string, n int) (string, error) {
	if n < 1 {
		n = 1
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	size := info.Size()
	if size == 0 {
		return "", nil
	}

	const chunk = 8192
	var buf []byte
	pos := size
	for pos > 0 && bytes.Count(buf, []byte{'\n'}) <= n {
		readSize := int64(chunk)
		if readSize > pos {
			readSize = pos
		}
		pos -= readSize
		tmp := make([]byte, readSize)
		if _, err := f.ReadAt(tmp, pos); err != nil && err != io.EOF {
			return "", err
		}
		buf = append(tmp, buf...)
	}

	if len(buf) > 0 && buf[len(buf)-1] == '\n' {
		buf = buf[:len(buf)-1]
	}
	lines := bytes.Split(buf, []byte{'\n'})
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return string(bytes.Join(lines, []byte{'\n'})), nil
}
