package apic

import "mem"
import "util"

type _oride_t struct {
	src int
	dst int
	// trigger sense
	level bool
	// polarity
	low bool
}

type acpi_ioapic_t struct {
	base      uintptr
	overrides map[int]_oride_t
}

func _acpi_cksum(tbl []uint8) {
	var cksum uint8
	for _, c := range tbl {
		cksum += c
	}
	if cksum != 0 {
		panic("bad ACPI table checksum")
	}
}

// returns a slice of the requested table and whether it was found
func _acpi_tbl(rsdt []uint8, sig string) ([]uint8, bool) {
	// RSDT contains 32bit pointers, XSDT contains 64bit pointers.
	hdrlen := 36
	ptrs := rsdt[hdrlen:]
	var tbl []uint8
	for len(ptrs) != 0 {
		tbln := mem.Pa_t(util.Readn(ptrs, 4, 0))
		ptrs = ptrs[4:]
		tbl = mem.Dmaplen(tbln, 8)
		if string(tbl[:4]) == sig {
			l := util.Readn(tbl, 4, 4)
			tbl = mem.Dmaplen(tbln, l)
			return tbl, true
		}
	}
	return nil, false
}

// returns number of cpus, IO physcal address of IO APIC, and whether both the
// number of CPUs and an IO APIC were found.
func _acpi_madt(rsdt []uint8) (int, acpi_ioapic_t, bool) {
	// find MADT table
	tbl, found := _acpi_tbl(rsdt, "APIC")
	var apicret acpi_ioapic_t
	if !found {
		return 0, apicret, false
	}
	_acpi_cksum(tbl)
	apicret.overrides = make(map[int]_oride_t)
	marrayoff := 44
	ncpu := 0
	elen := 1
	// m is array of "interrupt controller structures" in MADT
	for m := tbl[marrayoff:]; len(m) != 0; m = m[m[elen]:] {
		// ACPI 5.2.12.2: each processor is required to have a LAPIC
		// entry
		tlapic := uint8(0)
		tioapic := uint8(1)
		toverride := uint8(2)

		tiosapic := uint8(6)
		tlsapic := uint8(7)
		tpint := uint8(8)
		if m[0] == tlapic {
			flags := util.Readn(m, 4, 4)
			enabled := 1
			if flags&enabled != 0 {
				ncpu++
			}
		} else if m[0] == tioapic {
			apicret.base = uintptr(util.Readn(m, 4, 4))
			//fmt.Printf("IO APIC addr: %x\n", apicret.base)
			//fmt.Printf("IO APIC IRQ start: %v\n", Readn(m, 4, 8))
		} else if m[0] == toverride {
			src := util.Readn(m, 1, 3)
			dst := util.Readn(m, 4, 4)
			v := util.Readn(m, 2, 8)
			var nover _oride_t
			nover.src = src
			nover.dst = dst
			//var active string
			switch v & 0x3 {
			case 0:
				//active = "conforms"
				if dst < 16 {
					nover.low = true
				} else {
					nover.low = false
				}
			case 1:
				//active = "high"
				nover.low = false
			case 2:
				//active = "RESERVED?"
				panic("bad polarity")
			case 3:
				//active = "low"
				nover.low = true
			}
			//var trig string
			switch (v & 0xc) >> 2 {
			case 0:
				//trig = "conforms"
				if dst < 16 {
					nover.level = false
				} else {
					nover.level = true
				}
			case 1:
				//trig = "edge"
				nover.level = false
			case 2:
				//trig = "RESERVED?"
				panic("bad trigger mode")
			case 3:
				//trig = "level"
				nover.level = true
			}
			apicret.overrides[dst] = nover
			//fmt.Printf("IRQ OVERRIDE: %v -> %v (%v, %v)\n", src,
			//    dst, trig, active)
		} else if m[0] == tiosapic {
			//fmt.Printf("*** IO SAPIC\n")
		} else if m[0] == tlsapic {
			//fmt.Printf("*** LOCAL SAPIC\n")
		} else if m[0] == tpint {
			//fmt.Printf("*** PLATFORM INT\n")
		}
	}
	return ncpu, apicret, ncpu != 0 && apicret.base != 0
}

// returns false if ACPI claims that MSI is broken
func _acpi_fadt(rsdt []uint8) bool {
	tbl, found := _acpi_tbl(rsdt, "FACP")
	if !found {
		return false
	}
	_acpi_cksum(tbl)
	flags := util.Readn(tbl, 2, 109)
	nomsi := 1 << 3
	return flags&nomsi == 0
}

func _acpi_scan() ([]uint8, bool) {
	// ACPI 5.2.5: search for RSDP in EBDA and BIOS read-only memory
	ebdap := mem.Pa_t((0x40 << 4) | 0xe)
	p := mem.Physmem.Dmap8(ebdap)
	ebda := mem.Pa_t(util.Readn(p, 2, 0))
	ebda <<= 4

	isrsdp := func(d []uint8) bool {
		s := string(d[:8])
		if s != "RSD PTR " {
			return false
		}
		var cksum uint8
		for i := 0; i < 20; i++ {
			cksum += d[i]
		}
		if cksum != 0 {
			return false
		}
		return true
	}
	rsdplen := 36
	for i := mem.Pa_t(0); i < 1<<10; i += 16 {
		p = mem.Dmaplen(ebda+i, rsdplen)
		if isrsdp(p) {
			return p, true
		}
	}
	for bmem := mem.Pa_t(0xe0000); bmem < 0xfffff; bmem += 16 {
		p = mem.Dmaplen(bmem, rsdplen)
		if isrsdp(p) {
			return p, true
		}
	}
	return nil, false
}
