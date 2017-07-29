// Code generated by protoc-gen-go. DO NOT EDIT.
// source: core.proto

/*
Package packets is a generated protocol buffer package.

It is generated from these files:
	core.proto

It has these top-level messages:
	Packet
	Data
	Status
	Control
	Init
	Confirm
*/
package packets

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion2 // please upgrade the proto package

type Packet struct {
	// Types that are valid to be assigned to Body:
	//	*Packet_Init
	//	*Packet_Data
	//	*Packet_Status
	//	*Packet_Control
	//	*Packet_Confirm
	Body isPacket_Body `protobuf_oneof:"body"`
}

func (m *Packet) Reset()                    { *m = Packet{} }
func (m *Packet) String() string            { return proto.CompactTextString(m) }
func (*Packet) ProtoMessage()               {}
func (*Packet) Descriptor() ([]byte, []int) { return fileDescriptor0, []int{0} }

type isPacket_Body interface {
	isPacket_Body()
}

type Packet_Init struct {
	Init *Init `protobuf:"bytes,1,opt,name=init,oneof"`
}
type Packet_Data struct {
	Data *Data `protobuf:"bytes,2,opt,name=data,oneof"`
}
type Packet_Status struct {
	Status *Status `protobuf:"bytes,3,opt,name=status,oneof"`
}
type Packet_Control struct {
	Control *Control `protobuf:"bytes,4,opt,name=control,oneof"`
}
type Packet_Confirm struct {
	Confirm *Confirm `protobuf:"bytes,5,opt,name=confirm,oneof"`
}

func (*Packet_Init) isPacket_Body()    {}
func (*Packet_Data) isPacket_Body()    {}
func (*Packet_Status) isPacket_Body()  {}
func (*Packet_Control) isPacket_Body() {}
func (*Packet_Confirm) isPacket_Body() {}

func (m *Packet) GetBody() isPacket_Body {
	if m != nil {
		return m.Body
	}
	return nil
}

func (m *Packet) GetInit() *Init {
	if x, ok := m.GetBody().(*Packet_Init); ok {
		return x.Init
	}
	return nil
}

func (m *Packet) GetData() *Data {
	if x, ok := m.GetBody().(*Packet_Data); ok {
		return x.Data
	}
	return nil
}

func (m *Packet) GetStatus() *Status {
	if x, ok := m.GetBody().(*Packet_Status); ok {
		return x.Status
	}
	return nil
}

func (m *Packet) GetControl() *Control {
	if x, ok := m.GetBody().(*Packet_Control); ok {
		return x.Control
	}
	return nil
}

func (m *Packet) GetConfirm() *Confirm {
	if x, ok := m.GetBody().(*Packet_Confirm); ok {
		return x.Confirm
	}
	return nil
}

// XXX_OneofFuncs is for the internal use of the proto package.
func (*Packet) XXX_OneofFuncs() (func(msg proto.Message, b *proto.Buffer) error, func(msg proto.Message, tag, wire int, b *proto.Buffer) (bool, error), func(msg proto.Message) (n int), []interface{}) {
	return _Packet_OneofMarshaler, _Packet_OneofUnmarshaler, _Packet_OneofSizer, []interface{}{
		(*Packet_Init)(nil),
		(*Packet_Data)(nil),
		(*Packet_Status)(nil),
		(*Packet_Control)(nil),
		(*Packet_Confirm)(nil),
	}
}

func _Packet_OneofMarshaler(msg proto.Message, b *proto.Buffer) error {
	m := msg.(*Packet)
	// body
	switch x := m.Body.(type) {
	case *Packet_Init:
		b.EncodeVarint(1<<3 | proto.WireBytes)
		if err := b.EncodeMessage(x.Init); err != nil {
			return err
		}
	case *Packet_Data:
		b.EncodeVarint(2<<3 | proto.WireBytes)
		if err := b.EncodeMessage(x.Data); err != nil {
			return err
		}
	case *Packet_Status:
		b.EncodeVarint(3<<3 | proto.WireBytes)
		if err := b.EncodeMessage(x.Status); err != nil {
			return err
		}
	case *Packet_Control:
		b.EncodeVarint(4<<3 | proto.WireBytes)
		if err := b.EncodeMessage(x.Control); err != nil {
			return err
		}
	case *Packet_Confirm:
		b.EncodeVarint(5<<3 | proto.WireBytes)
		if err := b.EncodeMessage(x.Confirm); err != nil {
			return err
		}
	case nil:
	default:
		return fmt.Errorf("Packet.Body has unexpected type %T", x)
	}
	return nil
}

func _Packet_OneofUnmarshaler(msg proto.Message, tag, wire int, b *proto.Buffer) (bool, error) {
	m := msg.(*Packet)
	switch tag {
	case 1: // body.init
		if wire != proto.WireBytes {
			return true, proto.ErrInternalBadWireType
		}
		msg := new(Init)
		err := b.DecodeMessage(msg)
		m.Body = &Packet_Init{msg}
		return true, err
	case 2: // body.data
		if wire != proto.WireBytes {
			return true, proto.ErrInternalBadWireType
		}
		msg := new(Data)
		err := b.DecodeMessage(msg)
		m.Body = &Packet_Data{msg}
		return true, err
	case 3: // body.status
		if wire != proto.WireBytes {
			return true, proto.ErrInternalBadWireType
		}
		msg := new(Status)
		err := b.DecodeMessage(msg)
		m.Body = &Packet_Status{msg}
		return true, err
	case 4: // body.control
		if wire != proto.WireBytes {
			return true, proto.ErrInternalBadWireType
		}
		msg := new(Control)
		err := b.DecodeMessage(msg)
		m.Body = &Packet_Control{msg}
		return true, err
	case 5: // body.confirm
		if wire != proto.WireBytes {
			return true, proto.ErrInternalBadWireType
		}
		msg := new(Confirm)
		err := b.DecodeMessage(msg)
		m.Body = &Packet_Confirm{msg}
		return true, err
	default:
		return false, nil
	}
}

func _Packet_OneofSizer(msg proto.Message) (n int) {
	m := msg.(*Packet)
	// body
	switch x := m.Body.(type) {
	case *Packet_Init:
		s := proto.Size(x.Init)
		n += proto.SizeVarint(1<<3 | proto.WireBytes)
		n += proto.SizeVarint(uint64(s))
		n += s
	case *Packet_Data:
		s := proto.Size(x.Data)
		n += proto.SizeVarint(2<<3 | proto.WireBytes)
		n += proto.SizeVarint(uint64(s))
		n += s
	case *Packet_Status:
		s := proto.Size(x.Status)
		n += proto.SizeVarint(3<<3 | proto.WireBytes)
		n += proto.SizeVarint(uint64(s))
		n += s
	case *Packet_Control:
		s := proto.Size(x.Control)
		n += proto.SizeVarint(4<<3 | proto.WireBytes)
		n += proto.SizeVarint(uint64(s))
		n += s
	case *Packet_Confirm:
		s := proto.Size(x.Confirm)
		n += proto.SizeVarint(5<<3 | proto.WireBytes)
		n += proto.SizeVarint(uint64(s))
		n += s
	case nil:
	default:
		panic(fmt.Sprintf("proto: unexpected type %T in oneof", x))
	}
	return n
}

type Data struct {
	SequenceID uint32 `protobuf:"varint,1,opt,name=sequenceID" json:"sequenceID,omitempty"`
	Content    []byte `protobuf:"bytes,2,opt,name=content,proto3" json:"content,omitempty"`
}

func (m *Data) Reset()                    { *m = Data{} }
func (m *Data) String() string            { return proto.CompactTextString(m) }
func (*Data) ProtoMessage()               {}
func (*Data) Descriptor() ([]byte, []int) { return fileDescriptor0, []int{1} }

func (m *Data) GetSequenceID() uint32 {
	if m != nil {
		return m.SequenceID
	}
	return 0
}

func (m *Data) GetContent() []byte {
	if m != nil {
		return m.Content
	}
	return nil
}

type Status struct {
	Timestamp   int64    `protobuf:"varint,1,opt,name=timestamp" json:"timestamp,omitempty"`
	IncomingSeq uint32   `protobuf:"varint,2,opt,name=incomingSeq" json:"incomingSeq,omitempty"`
	Inflight    []uint32 `protobuf:"varint,3,rep,packed,name=inflight" json:"inflight,omitempty"`
}

func (m *Status) Reset()                    { *m = Status{} }
func (m *Status) String() string            { return proto.CompactTextString(m) }
func (*Status) ProtoMessage()               {}
func (*Status) Descriptor() ([]byte, []int) { return fileDescriptor0, []int{2} }

func (m *Status) GetTimestamp() int64 {
	if m != nil {
		return m.Timestamp
	}
	return 0
}

func (m *Status) GetIncomingSeq() uint32 {
	if m != nil {
		return m.IncomingSeq
	}
	return 0
}

func (m *Status) GetInflight() []uint32 {
	if m != nil {
		return m.Inflight
	}
	return nil
}

type Control struct {
	Timestamp int64 `protobuf:"varint,1,opt,name=timestamp" json:"timestamp,omitempty"`
	Redundant bool  `protobuf:"varint,3,opt,name=redundant" json:"redundant,omitempty"`
}

func (m *Control) Reset()                    { *m = Control{} }
func (m *Control) String() string            { return proto.CompactTextString(m) }
func (*Control) ProtoMessage()               {}
func (*Control) Descriptor() ([]byte, []int) { return fileDescriptor0, []int{3} }

func (m *Control) GetTimestamp() int64 {
	if m != nil {
		return m.Timestamp
	}
	return 0
}

func (m *Control) GetRedundant() bool {
	if m != nil {
		return m.Redundant
	}
	return false
}

type Init struct {
	Control   *Control `protobuf:"bytes,1,opt,name=control" json:"control,omitempty"`
	Interface uint64   `protobuf:"varint,2,opt,name=interface" json:"interface,omitempty"`
	Bandwidth uint32   `protobuf:"varint,3,opt,name=bandwidth" json:"bandwidth,omitempty"`
	Session   uint64   `protobuf:"varint,4,opt,name=session" json:"session,omitempty"`
}

func (m *Init) Reset()                    { *m = Init{} }
func (m *Init) String() string            { return proto.CompactTextString(m) }
func (*Init) ProtoMessage()               {}
func (*Init) Descriptor() ([]byte, []int) { return fileDescriptor0, []int{4} }

func (m *Init) GetControl() *Control {
	if m != nil {
		return m.Control
	}
	return nil
}

func (m *Init) GetInterface() uint64 {
	if m != nil {
		return m.Interface
	}
	return 0
}

func (m *Init) GetBandwidth() uint32 {
	if m != nil {
		return m.Bandwidth
	}
	return 0
}

func (m *Init) GetSession() uint64 {
	if m != nil {
		return m.Session
	}
	return 0
}

type Confirm struct {
	Session   uint64 `protobuf:"varint,1,opt,name=session" json:"session,omitempty"`
	Interface uint64 `protobuf:"varint,2,opt,name=interface" json:"interface,omitempty"`
}

func (m *Confirm) Reset()                    { *m = Confirm{} }
func (m *Confirm) String() string            { return proto.CompactTextString(m) }
func (*Confirm) ProtoMessage()               {}
func (*Confirm) Descriptor() ([]byte, []int) { return fileDescriptor0, []int{5} }

func (m *Confirm) GetSession() uint64 {
	if m != nil {
		return m.Session
	}
	return 0
}

func (m *Confirm) GetInterface() uint64 {
	if m != nil {
		return m.Interface
	}
	return 0
}

func init() {
	proto.RegisterType((*Packet)(nil), "packets.Packet")
	proto.RegisterType((*Data)(nil), "packets.Data")
	proto.RegisterType((*Status)(nil), "packets.Status")
	proto.RegisterType((*Control)(nil), "packets.Control")
	proto.RegisterType((*Init)(nil), "packets.Init")
	proto.RegisterType((*Confirm)(nil), "packets.Confirm")
}

func init() { proto.RegisterFile("core.proto", fileDescriptor0) }

var fileDescriptor0 = []byte{
	// 363 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x84, 0x92, 0xdb, 0x4e, 0xf2, 0x40,
	0x10, 0xc7, 0xe9, 0xd7, 0x7e, 0x05, 0x06, 0x1b, 0xcd, 0x5e, 0x35, 0x86, 0x18, 0x52, 0x6f, 0xd0,
	0x18, 0x2e, 0xf4, 0x05, 0x3c, 0x60, 0x02, 0x77, 0x66, 0x79, 0x82, 0xa5, 0xbb, 0xc0, 0x46, 0xba,
	0x0b, 0xdd, 0x21, 0xc6, 0x37, 0xf0, 0x35, 0x7d, 0x13, 0xb3, 0x43, 0xa1, 0x78, 0x88, 0xde, 0xed,
	0xfc, 0xe7, 0x97, 0x39, 0xfc, 0x67, 0x01, 0x72, 0x5b, 0xaa, 0xc1, 0xaa, 0xb4, 0x68, 0x59, 0x73,
	0x25, 0xf2, 0x67, 0x85, 0x2e, 0x7b, 0x0f, 0x20, 0x7e, 0xa2, 0x37, 0x3b, 0x87, 0x48, 0x1b, 0x8d,
	0x69, 0xd0, 0x0b, 0xfa, 0x9d, 0xeb, 0x64, 0x50, 0x21, 0x83, 0xb1, 0xd1, 0x38, 0x6a, 0x70, 0x4a,
	0x7a, 0x48, 0x0a, 0x14, 0xe9, 0xbf, 0x2f, 0xd0, 0x50, 0xa0, 0xf0, 0x90, 0x4f, 0xb2, 0x0b, 0x88,
	0x1d, 0x0a, 0xdc, 0xb8, 0x34, 0x24, 0xec, 0x78, 0x8f, 0x4d, 0x48, 0x1e, 0x35, 0x78, 0x05, 0xb0,
	0x2b, 0x68, 0xe6, 0xd6, 0x60, 0x69, 0x97, 0x69, 0x44, 0xec, 0xc9, 0x9e, 0x7d, 0xd8, 0xea, 0xa3,
	0x06, 0xdf, 0x21, 0x15, 0x3d, 0xd3, 0x65, 0x91, 0xfe, 0xff, 0x4e, 0x7b, 0xbd, 0xa2, 0xfd, 0xf3,
	0x3e, 0x86, 0x68, 0x6a, 0xe5, 0x6b, 0x76, 0x0b, 0x91, 0x1f, 0x8f, 0x9d, 0x01, 0x38, 0xb5, 0xde,
	0x28, 0x93, 0xab, 0xf1, 0x90, 0xd6, 0x4c, 0xf8, 0x81, 0xc2, 0xd2, 0xed, 0x2c, 0xca, 0x20, 0xad,
	0x77, 0xc4, 0x77, 0x61, 0x26, 0x21, 0xde, 0x4e, 0xce, 0xba, 0xd0, 0x46, 0x5d, 0x28, 0x87, 0xa2,
	0x58, 0x51, 0x89, 0x90, 0xd7, 0x02, 0xeb, 0x41, 0x47, 0x9b, 0xdc, 0x16, 0xda, 0xcc, 0x27, 0x6a,
	0x4d, 0x55, 0x12, 0x7e, 0x28, 0xb1, 0x53, 0x68, 0x69, 0x33, 0x5b, 0xea, 0xf9, 0x02, 0xd3, 0xb0,
	0x17, 0xf6, 0x13, 0xbe, 0x8f, 0xb3, 0x47, 0x68, 0x56, 0x3b, 0xff, 0xd1, 0xa6, 0x0b, 0xed, 0x52,
	0xc9, 0x8d, 0x91, 0xc2, 0x20, 0x59, 0xdc, 0xe2, 0xb5, 0x90, 0xbd, 0x05, 0x10, 0xf9, 0x9b, 0xb1,
	0xcb, 0xda, 0xdb, 0xe0, 0x67, 0x6f, 0x6b, 0x67, 0xbb, 0xd0, 0xd6, 0x06, 0x55, 0x39, 0x13, 0xb9,
	0xa2, 0xb9, 0x23, 0x5e, 0x0b, 0x3e, 0x3b, 0x15, 0x46, 0xbe, 0x68, 0x89, 0x0b, 0x6a, 0x98, 0xf0,
	0x5a, 0xf0, 0xbe, 0x39, 0xe5, 0x9c, 0xb6, 0x86, 0x6e, 0x18, 0xf1, 0x5d, 0x98, 0xdd, 0xd1, 0x46,
	0xfe, 0x18, 0x87, 0x50, 0xf0, 0x09, 0xfa, 0xbd, 0xf5, 0x34, 0xa6, 0x0f, 0x7b, 0xf3, 0x11, 0x00,
	0x00, 0xff, 0xff, 0xdc, 0xc2, 0x43, 0xc4, 0xbe, 0x02, 0x00, 0x00,
}
