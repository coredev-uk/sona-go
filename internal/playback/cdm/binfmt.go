package cdm

import (
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"fmt"
	"io"
	"os"
	"slices"
)

// binaryFormat checks that a library is a shared library for one
// platform: the static half of the load check, which also runs where no
// host can load the library (a download for another platform).
type binaryFormat func(r io.ReaderAt) error

var (
	elfX86_64   = elfFormat(elf.EM_X86_64)
	elfAArch64  = elfFormat(elf.EM_AARCH64)
	machoARM64  = machoFormat(macho.CpuArm64)
	machoX86_64 = machoFormat(macho.CpuAmd64)
	peAMD64     = peFormat(pe.IMAGE_FILE_MACHINE_AMD64)
	peARM64     = peFormat(pe.IMAGE_FILE_MACHINE_ARM64)
)

func elfFormat(m elf.Machine) binaryFormat {
	return func(r io.ReaderAt) error {
		f, err := elf.NewFile(r)
		if err != nil {
			return err
		}
		if f.Class != elf.ELFCLASS64 || f.Machine != m || f.Type != elf.ET_DYN {
			return fmt.Errorf("ELF %v %v %v, want a 64-bit %v shared object", f.Class, f.Machine, f.Type, m)
		}
		return nil
	}
}

func machoFormat(cpu macho.Cpu) binaryFormat {
	return func(r io.ReaderAt) error {
		check := func(f *macho.File) error {
			if f.Cpu != cpu || f.Type != macho.TypeDylib {
				return fmt.Errorf("Mach-O %v %v, want a %v dylib", f.Cpu, f.Type, cpu)
			}
			return nil
		}
		if fat, err := macho.NewFatFile(r); err == nil {
			i := slices.IndexFunc(fat.Arches, func(a macho.FatArch) bool { return a.Cpu == cpu })
			if i < 0 {
				return fmt.Errorf("universal Mach-O without %v", cpu)
			}
			return check(fat.Arches[i].File)
		}
		f, err := macho.NewFile(r)
		if err != nil {
			return err
		}
		return check(f)
	}
}

func peFormat(machine uint16) binaryFormat {
	return func(r io.ReaderAt) error {
		f, err := pe.NewFile(r)
		if err != nil {
			return err
		}
		if f.Machine != machine || f.Characteristics&pe.IMAGE_FILE_DLL == 0 {
			return fmt.Errorf("PE machine %#x characteristics %#x, want a %#x DLL", f.Machine, f.Characteristics, machine)
		}
		return nil
	}
}

// checkLibrary runs p's format check on the library at path.
func checkLibrary(path string, p Platform) error {
	f, err := os.Open(path)
	if err != nil {
		return &Error{Code: CodeDisk, Err: err}
	}
	defer func() { _ = f.Close() }()
	if err := p.format(f); err != nil {
		return fail(CodeArchive, "%s is not a %s library: %w", p.Lib, p, err)
	}
	return nil
}
