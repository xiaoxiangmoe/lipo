package lipo

import (
	"debug/macho"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"

	"github.com/konoui/lipo/pkg/lipo/lmacho"
	"github.com/konoui/lipo/pkg/util"
)

type fatArches []lmacho.FatArch

func (f fatArches) createFatBinary(path string, perm os.FileMode, cfg *lmacho.FatFileConfig) error {
	if len(f) == 0 {
		return errors.New("empty fat file due to no inputs")
	}

	out, err := createTemp(path)
	if err != nil {
		return err
	}
	// for an error in the middle of other operations
	defer out.Close()

	err = lmacho.NewFatFileFromArches(f, cfg).Create(out)
	if err != nil {
		return err
	}

	if err := out.Chmod(perm); err != nil {
		return err
	}

	if err := out.Close(); err != nil {
		return err
	}

	// atomic operation
	return os.Rename(out.Name(), path)
}

// createTemp creates a temporary file from file path
func createTemp(path string) (*os.File, error) {
	f, err := os.CreateTemp(filepath.Dir(path), "tmp-lipo-out")
	if err != nil {
		return nil, fmt.Errorf("can't create temporary output file: %w", err)
	}
	return f, nil
}

func (f fatArches) extract(arches ...string) fatArches {
	exist := util.ExistMap(arches, func(v string) string { return v })
	return util.Filter(f, func(v lmacho.FatArch) bool {
		_, ok := exist[lmacho.ToCpuString(v.Cpu, v.SubCpu)]
		return ok
	})
}

func (f fatArches) extractFamily(arches ...string) fatArches {
	exist := util.ExistMap(arches, func(v string) macho.Cpu {
		c, _, _ := lmacho.ToCpu(v)
		return c
	})
	return util.Filter(f, func(v lmacho.FatArch) bool {
		_, ok := exist[v.Cpu]
		return ok
	})
}

func (f fatArches) remove(arches ...string) fatArches {
	exist := util.ExistMap(arches, func(v string) string { return v })
	return util.Filter(f, func(v lmacho.FatArch) bool {
		_, ok := exist[lmacho.ToCpuString(v.Cpu, v.SubCpu)]
		return !ok
	})
}

func (f fatArches) contains(in fatArches) bool {
	arches := in.arches()
	return len(f.extract(arches...)) == len(in)
}

func (f fatArches) replace(with fatArches) fatArches {
	arches := with.arches()
	new := f.remove(arches...)
	return append(new, with...)
}

func (f fatArches) arches() []string {
	return util.Map(f, func(v lmacho.FatArch) string {
		return lmacho.ToCpuString(v.Cpu, v.SubCpu)
	})
}

func (f fatArches) updateAlignBit(segAligns []*SegAlignInput) error {
	if len(segAligns) == 0 {
		return nil
	}

	dup := util.Duplicates(segAligns, func(k *SegAlignInput) string { return k.Arch })
	if dup != nil {
		return fmt.Errorf("segalign %s specified multiple times", *dup)
	}

	for _, a := range segAligns {
		align, err := strconv.ParseInt(a.AlignHex, 16, 64)
		if err != nil {
			return err
		}

		if align == 0 || (align != 1 && (align%2) != 0) {
			return fmt.Errorf("segalign %s (hex) must be a non-zero power of two", a.AlignHex)
		}

		alignBit := uint32(math.Log2(float64(align)))
		found := false
		for idx := range f {
			if lmacho.ToCpuString(f[idx].Cpu, f[idx].SubCpu) == a.Arch {
				f[idx].Align = alignBit
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("segalign %s specified but resulting fat file does not contain that architecture", a.Arch)
		}
	}

	return nil
}
