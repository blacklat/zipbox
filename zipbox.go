package zipbox

import (
	"os"
	"sync"
	"bytes"
	"fmt"
	"strings"
	"io"
	"io/ioutil"
	"archive/zip"
	"path/filepath"
	"time"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"net/http"
)

type ZipBox struct {
	name    string
	absPath string
	box     *zipBox
}

type zipBox struct {
	Name  string
	Files map[string]*zipBoxFile
	Time  time.Time
}

type zipBoxDir struct {
	name string
	time time.Time
}

type zipBoxFile struct {
	zipFile  *zip.File
	dir      bool
	dirInfo  *zipBoxDir
	children []*zipBoxFile
	content  []byte
	reader  *bytes.Reader
}

type File struct {
	*zipBoxFile
}

type HTTPZipBox struct {
	*ZipBox
}

var zipBoxes sync.Map

func init() {
	var exeFile string
	if file, err := os.Executable(); err != nil {
		return
	} else {
		if filePath, err := filepath.EvalSymlinks(file); err != nil {
			return
		} else {
			exeFile = filePath
		}
	}
	closer, reader, err := OpenZipExe(exeFile)
	if err != nil {
		return
	}
	defer closer.Close()
	for _, file := range reader.File {
		fileParts := strings.SplitN(strings.TrimLeft(filepath.ToSlash(file.Name), "/"), "/", 2)
		boxName := fileParts[0]
		var fileName string
		if len(fileParts) > 1 {
			fileName = fileParts[1]
		}
		box, _ := zipBoxes.LoadOrStore(boxName, &zipBox{
			Name:  boxName,
			Files: make(map[string]*zipBoxFile),
			Time:  file.ModTime(),
		})
		zboxFile := &zipBoxFile{zipFile: file}
		if file.Comment == "dir" {
			zboxFile.dir = true
			zboxFile.dirInfo = &zipBoxDir{
				name: filepath.Base(zboxFile.zipFile.Name),
				time: zboxFile.zipFile.ModTime(),
			}
		} else {
			zboxFile.content = make([]byte, zboxFile.zipFile.FileInfo().Size())
			if len(zboxFile.content) > 0 {
				rc, err := zboxFile.zipFile.Open()
				if err != nil {
					zboxFile.content = nil
					fmt.Printf("error opening appended file %s: %v\n", zboxFile.zipFile.Name, err)
				} else {
					if _, err := rc.Read(zboxFile.content); err != nil {
						zboxFile.content = nil
						fmt.Printf("error reading data for appended file %s: %v\n", zboxFile.zipFile.Name, err)
					}
					rc.Close()
				}
			}
		}
		box.(*zipBox).Files[fileName] = zboxFile
		dirName := filepath.Dir(fileName)
		if dirName == "." {
			dirName = ""
		}
		if fileName != "" {
			if dir := box.(*zipBox).Files[dirName]; dir != nil {
				dir.children = append(dir.children, zboxFile)
			}
		}
	}
}

func get(name string) (*ZipBox, error) {
	z := &ZipBox{name: name}
	if filepath.IsAbs(name) {
		return nil, fmt.Errorf("given name/path is absolute")
	}
	zboxName := strings.Replace(name, `/`, `-`, -1)
	if box, ok := zipBoxes.Load(zboxName); ok {
		z.box = box.(*zipBox)
		return z, nil
	}
	return nil, fmt.Errorf("unable to find zipbox %q", name)
}

func Get(name string) (*ZipBox, error) {
	return get(name)
}

func (z *ZipBox) Time() time.Time {
	return z.box.Time
}

func (z *ZipBox) Open(name string) (*File, error) {
	name = strings.TrimLeft(name, "/")
	zboxFile := z.box.Files[name]
	if zboxFile == nil {
		return nil, &os.PathError{
			Op:   "open",
			Path: name,
			Err:  os.ErrNotExist,
		}
	}
	f := &File{zboxFile}
	if !zboxFile.dir {
		if zboxFile.content == nil {
			return nil, &os.PathError{
				Op:   "open",
				Path: "name",
				Err:  fmt.Errorf("error reading data from zip file"),
			}
		}
		f.reader = bytes.NewReader(zboxFile.content)
	}
	return f, nil
}

func (z *ZipBox) Bytes(file string) ([]byte, error) {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (z *ZipBox) String(name string) (string, error) {
	data, err := z.Bytes(name)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (z *ZipBox) Name() string {
	return z.name
}

func (z *zipBoxDir) Name() string {
        return z.name
}
func (z *zipBoxDir) Size() int64 {
        return 0
}
func (z *zipBoxDir) Mode() os.FileMode {
        return os.ModeDir
}
func (z *zipBoxDir) ModTime() time.Time {
        return z.time
}
func (z *zipBoxDir) IsDir() bool {
        return true
}
func (z *zipBoxDir) Sys() interface{} {
        return nil
}

func OpenZipExe(path string) (io.Closer, *zip.Reader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil {
		return nil, nil, err
	}
	reader, err := NewReader(file, info.Size())
	if err != nil {
		return nil, nil, err
	}
	return file, reader, nil
}

func NewReader(reader io.ReaderAt, size int64) (*zip.Reader, error) {
	handlers := []func(io.ReaderAt, int64) (*zip.Reader, error){
		zip.NewReader,
		zipExeReaderMacho,
		zipExeReaderElf,
		zipExeReaderPe,
	}
	for _, handler := range handlers {
		if file, err := handler(reader, size); err == nil {
			return file, nil
		}
	}
	return nil, fmt.Errorf("unable to open as executable")
}

func zipExeReaderMacho(reader io.ReaderAt, size int64) (*zip.Reader, error) {
	file, err := macho.NewFile(reader)
	if err != nil {
		return nil, err
	}
	var max int64
	for _, load := range file.Loads {
		if seg, ok := load.(*macho.Segment); ok {
			if zfile, err := zip.NewReader(seg, int64(seg.Filesz)); err == nil {
				return zfile, nil
			}
			end := int64(seg.Offset + seg.Filesz)
			if end > max {
				max = end
			}
		}
	}
	section := io.NewSectionReader(reader, max, size-max)
	return zip.NewReader(section, section.Size())
}

func zipExeReaderPe(reader io.ReaderAt, size int64) (*zip.Reader, error) {
	file, err := pe.NewFile(reader)
	if err != nil {
		return nil, err
	}
	var max int64
	for _, sec := range file.Sections {
		if zfile, err := zip.NewReader(sec, int64(sec.Size)); err == nil {
			return zfile, nil
		}
		end := int64(sec.Offset + sec.Size)
		if end > max {
			max = end
		}
	}
	section := io.NewSectionReader(reader, max, size-max)
	return zip.NewReader(section, section.Size())
}

func zipExeReaderElf(reader io.ReaderAt, size int64) (*zip.Reader, error) {
	file, err := elf.NewFile(reader)
	if err != nil {
		return nil, err
	}
	var max int64
	for _, sect := range file.Sections {
		if sect.Type == elf.SHT_NOBITS {
			continue
		}
		if zfile, err := zip.NewReader(sect, int64(sect.Size)); err == nil {
			return zfile, nil
		}
		end := int64(sect.Offset + sect.Size)
		if end > max {
			max = end
		}
	}
	section := io.NewSectionReader(reader, max, size-max)
	return zip.NewReader(section, section.Size())
}

func (z *ZipBox) HTTPZipBox() *HTTPZipBox {
	return &HTTPZipBox{z}
}

func (z *HTTPZipBox) Open(name string) (http.File, error) {
	return z.ZipBox.Open(name)
}

func (f *File) Close() error {
	if f.reader == nil {
		return fmt.Errorf("already closed")
	}
	f.reader = nil
	return nil
}

func (f *File) Stat() (os.FileInfo, error) {
	if f.dir {
		return f.dirInfo, nil
	}
	if f.reader == nil {
		return nil, fmt.Errorf("file is closed")
	}
	return f.zipFile.FileInfo(), nil
}

func (f *File) Readdir(count int) ([]os.FileInfo, error) {
	if f.dir {
		fi := make([]os.FileInfo, 0, len(f.children))
		for _, child := range f.children {
			if child.dir {
				fi = append(fi, child.dirInfo)
			} else {
				fi = append(fi, child.zipFile.FileInfo())
			}
		}
		return fi, nil
	}
	return nil, os.ErrInvalid
}

func (f *File) Readdirnames(count int) ([]string, error) {
	if f.dir {
		names := make([]string, 0, len(f.children))
		for _, child := range f.children {
			if child.dir {
				names = append(names, child.dirInfo.name)
			} else {
				names = append(names, child.zipFile.FileInfo().Name())
			}
		}
		return names, nil
	}
	return nil, os.ErrInvalid
}

func (f *File) Read(bts []byte) (int, error) {
	if f.reader == nil {
		return 0, &os.PathError{
			Op:   "read",
			Path: filepath.Base(f.zipFile.Name),
			Err:  fmt.Errorf("file is closed"),
		}
	}
	if f.dir {
		return 0, &os.PathError{
			Op:   "read",
			Path: filepath.Base(f.zipFile.Name),
			Err:  fmt.Errorf("is a directory"),
		}
	}
	return f.reader.Read(bts)
}

func (f *File) Seek(offset int64, whence int) (int64, error) {
	if f.reader == nil {
		return 0, &os.PathError{
			Op:   "seek",
			Path: filepath.Base(f.zipFile.Name),
			Err:  fmt.Errorf("file is closed"),
		}
	}
	return f.reader.Seek(offset, whence)
}

