package load

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha1" //nolint:gosec // git's object names, not a security check
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/farzeni/skl/internal/vault"
)

const (
	// manifestFile is the record each loaded copy holds. It is never part of the
	// copy's content.
	manifestFile = ".skl-copy.toml"
	// originFile stays in the vault: it isn't copied into a loaded skill.
	originFile = ".skl-origin.toml"
)

// Entry kinds that git keeps. Everything else on disk, such as a pipe, it ignores.
const (
	modeFile    = "100644"
	modeExec    = "100755"
	modeSymlink = "120000"
	modeFolder  = "40000"
)

// entryMode is the mode git would give this entry, or "" if git ignores it. Git
// keeps only whether the owner may execute a file.
func entryMode(e fs.DirEntry) (string, error) {
	switch t := e.Type(); {
	case t.IsDir():
		return modeFolder, nil
	case t&fs.ModeSymlink != 0:
		return modeSymlink, nil
	case !t.IsRegular():
		return "", nil
	}
	info, err := e.Info()
	if err != nil {
		return "", err
	}
	if info.Mode().Perm()&0o100 != 0 {
		return modeExec, nil
	}
	return modeFile, nil
}

// ignoredAt reports the entries of a skill's folder that aren't its content: git's
// own folder at any depth, and skl's records at the top. depth 0 is the top.
func ignoredAt(name string, depth int, skip []string) bool {
	if strings.EqualFold(name, ".git") {
		return true
	}
	return depth == 0 && slices.Contains(skip, name)
}

// Tree returns the git tree hash of a skill's folder, without its .skl-copy.toml:
// what git write-tree gives for the same files, modes and bytes, offline and with
// no index. Like git, it keeps no empty folders and only the executable bit.
func Tree(ctx context.Context, dir string) (string, error) {
	return treeOf(ctx, dir, manifestFile)
}

// treeOf hashes dir without the named entries at its top.
func treeOf(ctx context.Context, dir string, skip ...string) (string, error) {
	sum, err := hashFolder(ctx, dir, "", 0, skip, nil)
	if err != nil {
		return "", err
	}
	if sum == nil {
		sum = gitObject("tree", nil)
	}
	return hex.EncodeToString(sum), nil
}

// hashFolder returns the tree object's name, or nil for a folder git wouldn't keep.
// rel is dir's path inside the folder being hashed. If snap isn't nil, every object
// is added to it.
func hashFolder(ctx context.Context, dir, rel string, depth int, skip []string, snap *snapshot) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	type treeEntry struct {
		mode, name string
		sum        []byte
	}
	var kept []treeEntry
	for _, e := range entries {
		if ignoredAt(e.Name(), depth, skip) {
			continue
		}
		mode, err := entryMode(e)
		if err != nil {
			return nil, err
		}
		file, name := filepath.Join(dir, e.Name()), path.Join(rel, e.Name())
		var sum []byte
		switch mode {
		case "":
			continue
		case modeFolder:
			if sum, err = hashFolder(ctx, file, name, depth+1, skip, snap); err == nil && sum == nil {
				continue
			}
		case modeSymlink:
			sum, err = hashLink(file)
			snap.link(file, name, sum)
		default:
			sum, err = hashFile(file)
			snap.file(file, name, mode, sum)
		}
		if err != nil {
			return nil, err
		}
		kept = append(kept, treeEntry{mode, e.Name(), sum})
	}
	if len(kept) == 0 {
		return nil, nil
	}
	// Git sorts a folder as if its name ended in "/".
	key := func(e treeEntry) string {
		if e.mode == modeFolder {
			return e.name + "/"
		}
		return e.name
	}
	slices.SortFunc(kept, func(a, b treeEntry) int { return cmp.Compare(key(a), key(b)) })
	var body bytes.Buffer
	for _, e := range kept {
		fmt.Fprintf(&body, "%s %s\x00", e.mode, e.name)
		body.Write(e.sum)
	}
	sum := gitObject("tree", body.Bytes())
	if snap != nil {
		var text strings.Builder // the same tree, as mktree -z reads it
		for _, e := range kept {
			kind := "blob"
			if e.mode == modeFolder {
				kind = "tree"
			}
			fmt.Fprintf(&text, "%s %s %s\t%s\x00", e.mode, kind, hex.EncodeToString(e.sum), e.name)
		}
		snap.objects.Trees = append(snap.objects.Trees, text.String())
		snap.treeSums = append(snap.treeSums, hex.EncodeToString(sum))
	}
	return sum, nil
}

// snapshot is a folder's git objects, for writing it into the vault. Its methods
// do nothing on a nil snapshot, so plain hashing collects nothing.
type snapshot struct {
	tree    string
	objects vault.Objects
	fileSums, linkSums, treeSums []string // the objects' names, in the order of objects
	files   map[string]vault.File // each file and link, by path inside the folder
}

func (s *snapshot) file(file, name, mode string, sum []byte) {
	if s == nil || sum == nil {
		return
	}
	s.objects.Files = append(s.objects.Files, file)
	s.fileSums = append(s.fileSums, hex.EncodeToString(sum))
	s.files[name] = vault.File{Mode: mode, Object: hex.EncodeToString(sum)}
}

func (s *snapshot) link(file, name string, sum []byte) {
	if s == nil || sum == nil {
		return
	}
	target, err := os.Readlink(file)
	if err != nil {
		return // hashLink read it a moment ago; the next step reports it if it's gone
	}
	s.objects.Links = append(s.objects.Links, target)
	s.linkSums = append(s.linkSums, hex.EncodeToString(sum))
	s.files[name] = vault.File{Mode: modeSymlink, Object: hex.EncodeToString(sum)}
}

// snapshotOf hashes a copy's folder, without its .skl-copy.toml, and collects its
// objects.
func snapshotOf(ctx context.Context, dir string) (*snapshot, error) {
	s := &snapshot{files: map[string]vault.File{}}
	sum, err := hashFolder(ctx, dir, "", 0, []string{manifestFile}, s)
	if err != nil {
		return nil, err
	}
	if sum != nil {
		s.tree = hex.EncodeToString(sum)
	}
	return s, nil
}

func gitObject(kind string, body []byte) []byte {
	h := sha1.New() //nolint:gosec // git's object names
	h.Write([]byte(kind + " " + strconv.Itoa(len(body)) + "\x00"))
	h.Write(body)
	return h.Sum(nil)
}

func hashLink(path string) ([]byte, error) {
	target, err := os.Readlink(path)
	if err != nil {
		return nil, err
	}
	return gitObject("blob", []byte(target)), nil
}

func hashFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	h := sha1.New() //nolint:gosec // git's object names
	h.Write([]byte("blob " + strconv.FormatInt(info.Size(), 10) + "\x00"))
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

// copyFolder writes src's content to dst the way git would check it out: the same
// bytes, links as links, 644 or 755, no empty folders. dst must not exist.
func copyFolder(ctx context.Context, src, dst string, depth int, skip []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if ignoredAt(e.Name(), depth, skip) {
			continue
		}
		mode, err := entryMode(e)
		if err != nil {
			return err
		}
		from, to := filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())
		switch mode {
		case "":
		case modeFolder:
			err = copyFolder(ctx, from, to, depth+1, skip)
		case modeSymlink:
			err = copyLink(from, to)
		case modeExec:
			err = copyFile(from, to, 0o755)
		default:
			err = copyFile(from, to, 0o644)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func copyLink(from, to string) error {
	target, err := os.Readlink(from)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	return os.Symlink(target, to)
}

func copyFile(from, to string, perm fs.FileMode) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}
