package virtcli

import "encoding/xml"

// DomainXML is a minimal projection of libvirt's domain XML covering the
// fields vm-info needs (devices/disks/interfaces).
type DomainXML struct {
	XMLName xml.Name      `xml:"domain"`
	Name    string        `xml:"name"`
	UUID    string        `xml:"uuid"`
	Memory  MemoryElement `xml:"memory"`
	CurMem  MemoryElement `xml:"currentMemory"`
	VCPU    VCPUElement   `xml:"vcpu"`
	Devices Devices       `xml:"devices"`
}

type MemoryElement struct {
	Unit  string `xml:"unit,attr"`
	Value uint64 `xml:",chardata"`
}

type VCPUElement struct {
	Current string `xml:"current,attr"`
	Value   int    `xml:",chardata"`
}

type Devices struct {
	Disks      []Disk      `xml:"disk"`
	Interfaces []Interface `xml:"interface"`
}

type Disk struct {
	Type   string     `xml:"type,attr"`
	Device string     `xml:"device,attr"`
	Driver DiskDriver `xml:"driver"`
	Source DiskSource `xml:"source"`
	Target DiskTarget `xml:"target"`
}

type DiskDriver struct {
	Name string `xml:"name,attr"`
	Type string `xml:"type,attr"`
}

type DiskSource struct {
	File   string `xml:"file,attr"`
	Dev    string `xml:"dev,attr"`
	Pool   string `xml:"pool,attr"`
	Volume string `xml:"volume,attr"`
	Name   string `xml:"name,attr"`
}

type DiskTarget struct {
	Dev string `xml:"dev,attr"`
	Bus string `xml:"bus,attr"`
}

func (d Disk) SourcePath() string {
	switch {
	case d.Source.File != "":
		return d.Source.File
	case d.Source.Dev != "":
		return d.Source.Dev
	case d.Source.Pool != "" && d.Source.Volume != "":
		return d.Source.Pool + "/" + d.Source.Volume
	case d.Source.Name != "":
		return d.Source.Name
	}
	return ""
}

type Interface struct {
	Type   string          `xml:"type,attr"`
	MAC    InterfaceMAC    `xml:"mac"`
	Source InterfaceSource `xml:"source"`
	Model  InterfaceModel  `xml:"model"`
	Target InterfaceTarget `xml:"target"`
}

type InterfaceMAC struct {
	Address string `xml:"address,attr"`
}

type InterfaceSource struct {
	Network string `xml:"network,attr"`
	Bridge  string `xml:"bridge,attr"`
	Dev     string `xml:"dev,attr"`
	Mode    string `xml:"mode,attr"`
}

type InterfaceModel struct {
	Type string `xml:"type,attr"`
}

type InterfaceTarget struct {
	Dev string `xml:"dev,attr"`
}

func (i Interface) SourceLabel() string {
	switch i.Type {
	case "network":
		if i.Source.Network != "" {
			return "network=" + i.Source.Network
		}
	case "bridge":
		if i.Source.Bridge != "" {
			return "bridge=" + i.Source.Bridge
		}
	case "direct":
		if i.Source.Dev != "" {
			return "direct=" + i.Source.Dev
		}
	}
	if i.Source.Network != "" {
		return i.Source.Network
	}
	if i.Source.Bridge != "" {
		return i.Source.Bridge
	}
	if i.Source.Dev != "" {
		return i.Source.Dev
	}
	return "-"
}

func ParseDomainXML(s string) (*DomainXML, error) {
	var d DomainXML
	if err := xml.Unmarshal([]byte(s), &d); err != nil {
		return nil, err
	}
	return &d, nil
}
