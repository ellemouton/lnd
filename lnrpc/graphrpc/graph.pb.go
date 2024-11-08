// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.33.0
// 	protoc        v3.21.12
// source: graphrpc/graph.proto

package graphrpc

import (
	lnrpc "github.com/lightningnetwork/lnd/lnrpc"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type BetweennessCentralityReq struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *BetweennessCentralityReq) Reset() {
	*x = BetweennessCentralityReq{}
	if protoimpl.UnsafeEnabled {
		mi := &file_graphrpc_graph_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *BetweennessCentralityReq) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*BetweennessCentralityReq) ProtoMessage() {}

func (x *BetweennessCentralityReq) ProtoReflect() protoreflect.Message {
	mi := &file_graphrpc_graph_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use BetweennessCentralityReq.ProtoReflect.Descriptor instead.
func (*BetweennessCentralityReq) Descriptor() ([]byte, []int) {
	return file_graphrpc_graph_proto_rawDescGZIP(), []int{0}
}

type BetweennessCentralityResp struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	NodeBetweeness []*BetweenessCentrality `protobuf:"bytes,1,rep,name=node_betweeness,json=nodeBetweeness,proto3" json:"node_betweeness,omitempty"`
}

func (x *BetweennessCentralityResp) Reset() {
	*x = BetweennessCentralityResp{}
	if protoimpl.UnsafeEnabled {
		mi := &file_graphrpc_graph_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *BetweennessCentralityResp) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*BetweennessCentralityResp) ProtoMessage() {}

func (x *BetweennessCentralityResp) ProtoReflect() protoreflect.Message {
	mi := &file_graphrpc_graph_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use BetweennessCentralityResp.ProtoReflect.Descriptor instead.
func (*BetweennessCentralityResp) Descriptor() ([]byte, []int) {
	return file_graphrpc_graph_proto_rawDescGZIP(), []int{1}
}

func (x *BetweennessCentralityResp) GetNodeBetweeness() []*BetweenessCentrality {
	if x != nil {
		return x.NodeBetweeness
	}
	return nil
}

type BetweenessCentrality struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Node          []byte  `protobuf:"bytes,1,opt,name=node,proto3" json:"node,omitempty"`
	Normalized    float32 `protobuf:"fixed32,2,opt,name=normalized,proto3" json:"normalized,omitempty"`
	NonNormalized float32 `protobuf:"fixed32,3,opt,name=non_normalized,json=nonNormalized,proto3" json:"non_normalized,omitempty"`
}

func (x *BetweenessCentrality) Reset() {
	*x = BetweenessCentrality{}
	if protoimpl.UnsafeEnabled {
		mi := &file_graphrpc_graph_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *BetweenessCentrality) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*BetweenessCentrality) ProtoMessage() {}

func (x *BetweenessCentrality) ProtoReflect() protoreflect.Message {
	mi := &file_graphrpc_graph_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use BetweenessCentrality.ProtoReflect.Descriptor instead.
func (*BetweenessCentrality) Descriptor() ([]byte, []int) {
	return file_graphrpc_graph_proto_rawDescGZIP(), []int{2}
}

func (x *BetweenessCentrality) GetNode() []byte {
	if x != nil {
		return x.Node
	}
	return nil
}

func (x *BetweenessCentrality) GetNormalized() float32 {
	if x != nil {
		return x.Normalized
	}
	return 0
}

func (x *BetweenessCentrality) GetNonNormalized() float32 {
	if x != nil {
		return x.NonNormalized
	}
	return 0
}

type BootstrapAddrsReq struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	NumAddrs    uint32   `protobuf:"varint,1,opt,name=num_addrs,json=numAddrs,proto3" json:"num_addrs,omitempty"`
	IgnoreNodes [][]byte `protobuf:"bytes,2,rep,name=ignore_nodes,json=ignoreNodes,proto3" json:"ignore_nodes,omitempty"`
}

func (x *BootstrapAddrsReq) Reset() {
	*x = BootstrapAddrsReq{}
	if protoimpl.UnsafeEnabled {
		mi := &file_graphrpc_graph_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *BootstrapAddrsReq) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*BootstrapAddrsReq) ProtoMessage() {}

func (x *BootstrapAddrsReq) ProtoReflect() protoreflect.Message {
	mi := &file_graphrpc_graph_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use BootstrapAddrsReq.ProtoReflect.Descriptor instead.
func (*BootstrapAddrsReq) Descriptor() ([]byte, []int) {
	return file_graphrpc_graph_proto_rawDescGZIP(), []int{3}
}

func (x *BootstrapAddrsReq) GetNumAddrs() uint32 {
	if x != nil {
		return x.NumAddrs
	}
	return 0
}

func (x *BootstrapAddrsReq) GetIgnoreNodes() [][]byte {
	if x != nil {
		return x.IgnoreNodes
	}
	return nil
}

type BootstrapAddrsResp struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Addrs []*NetAddress `protobuf:"bytes,1,rep,name=addrs,proto3" json:"addrs,omitempty"`
}

func (x *BootstrapAddrsResp) Reset() {
	*x = BootstrapAddrsResp{}
	if protoimpl.UnsafeEnabled {
		mi := &file_graphrpc_graph_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *BootstrapAddrsResp) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*BootstrapAddrsResp) ProtoMessage() {}

func (x *BootstrapAddrsResp) ProtoReflect() protoreflect.Message {
	mi := &file_graphrpc_graph_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use BootstrapAddrsResp.ProtoReflect.Descriptor instead.
func (*BootstrapAddrsResp) Descriptor() ([]byte, []int) {
	return file_graphrpc_graph_proto_rawDescGZIP(), []int{4}
}

func (x *BootstrapAddrsResp) GetAddrs() []*NetAddress {
	if x != nil {
		return x.Addrs
	}
	return nil
}

type NetAddress struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	NodeId  []byte             `protobuf:"bytes,1,opt,name=node_id,json=nodeId,proto3" json:"node_id,omitempty"`
	Address *lnrpc.NodeAddress `protobuf:"bytes,2,opt,name=address,proto3" json:"address,omitempty"`
}

func (x *NetAddress) Reset() {
	*x = NetAddress{}
	if protoimpl.UnsafeEnabled {
		mi := &file_graphrpc_graph_proto_msgTypes[5]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *NetAddress) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*NetAddress) ProtoMessage() {}

func (x *NetAddress) ProtoReflect() protoreflect.Message {
	mi := &file_graphrpc_graph_proto_msgTypes[5]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use NetAddress.ProtoReflect.Descriptor instead.
func (*NetAddress) Descriptor() ([]byte, []int) {
	return file_graphrpc_graph_proto_rawDescGZIP(), []int{5}
}

func (x *NetAddress) GetNodeId() []byte {
	if x != nil {
		return x.NodeId
	}
	return nil
}

func (x *NetAddress) GetAddress() *lnrpc.NodeAddress {
	if x != nil {
		return x.Address
	}
	return nil
}

type BoostrapperNameReq struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *BoostrapperNameReq) Reset() {
	*x = BoostrapperNameReq{}
	if protoimpl.UnsafeEnabled {
		mi := &file_graphrpc_graph_proto_msgTypes[6]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *BoostrapperNameReq) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*BoostrapperNameReq) ProtoMessage() {}

func (x *BoostrapperNameReq) ProtoReflect() protoreflect.Message {
	mi := &file_graphrpc_graph_proto_msgTypes[6]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use BoostrapperNameReq.ProtoReflect.Descriptor instead.
func (*BoostrapperNameReq) Descriptor() ([]byte, []int) {
	return file_graphrpc_graph_proto_rawDescGZIP(), []int{6}
}

type BoostrapperNameResp struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
}

func (x *BoostrapperNameResp) Reset() {
	*x = BoostrapperNameResp{}
	if protoimpl.UnsafeEnabled {
		mi := &file_graphrpc_graph_proto_msgTypes[7]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *BoostrapperNameResp) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*BoostrapperNameResp) ProtoMessage() {}

func (x *BoostrapperNameResp) ProtoReflect() protoreflect.Message {
	mi := &file_graphrpc_graph_proto_msgTypes[7]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use BoostrapperNameResp.ProtoReflect.Descriptor instead.
func (*BoostrapperNameResp) Descriptor() ([]byte, []int) {
	return file_graphrpc_graph_proto_rawDescGZIP(), []int{7}
}

func (x *BoostrapperNameResp) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

var File_graphrpc_graph_proto protoreflect.FileDescriptor

var file_graphrpc_graph_proto_rawDesc = []byte{
	0x0a, 0x14, 0x67, 0x72, 0x61, 0x70, 0x68, 0x72, 0x70, 0x63, 0x2f, 0x67, 0x72, 0x61, 0x70, 0x68,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x08, 0x67, 0x72, 0x61, 0x70, 0x68, 0x72, 0x70, 0x63,
	0x1a, 0x0f, 0x6c, 0x69, 0x67, 0x68, 0x74, 0x6e, 0x69, 0x6e, 0x67, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x22, 0x1a, 0x0a, 0x18, 0x42, 0x65, 0x74, 0x77, 0x65, 0x65, 0x6e, 0x6e, 0x65, 0x73, 0x73,
	0x43, 0x65, 0x6e, 0x74, 0x72, 0x61, 0x6c, 0x69, 0x74, 0x79, 0x52, 0x65, 0x71, 0x22, 0x64, 0x0a,
	0x19, 0x42, 0x65, 0x74, 0x77, 0x65, 0x65, 0x6e, 0x6e, 0x65, 0x73, 0x73, 0x43, 0x65, 0x6e, 0x74,
	0x72, 0x61, 0x6c, 0x69, 0x74, 0x79, 0x52, 0x65, 0x73, 0x70, 0x12, 0x47, 0x0a, 0x0f, 0x6e, 0x6f,
	0x64, 0x65, 0x5f, 0x62, 0x65, 0x74, 0x77, 0x65, 0x65, 0x6e, 0x65, 0x73, 0x73, 0x18, 0x01, 0x20,
	0x03, 0x28, 0x0b, 0x32, 0x1e, 0x2e, 0x67, 0x72, 0x61, 0x70, 0x68, 0x72, 0x70, 0x63, 0x2e, 0x42,
	0x65, 0x74, 0x77, 0x65, 0x65, 0x6e, 0x65, 0x73, 0x73, 0x43, 0x65, 0x6e, 0x74, 0x72, 0x61, 0x6c,
	0x69, 0x74, 0x79, 0x52, 0x0e, 0x6e, 0x6f, 0x64, 0x65, 0x42, 0x65, 0x74, 0x77, 0x65, 0x65, 0x6e,
	0x65, 0x73, 0x73, 0x22, 0x71, 0x0a, 0x14, 0x42, 0x65, 0x74, 0x77, 0x65, 0x65, 0x6e, 0x65, 0x73,
	0x73, 0x43, 0x65, 0x6e, 0x74, 0x72, 0x61, 0x6c, 0x69, 0x74, 0x79, 0x12, 0x12, 0x0a, 0x04, 0x6e,
	0x6f, 0x64, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x04, 0x6e, 0x6f, 0x64, 0x65, 0x12,
	0x1e, 0x0a, 0x0a, 0x6e, 0x6f, 0x72, 0x6d, 0x61, 0x6c, 0x69, 0x7a, 0x65, 0x64, 0x18, 0x02, 0x20,
	0x01, 0x28, 0x02, 0x52, 0x0a, 0x6e, 0x6f, 0x72, 0x6d, 0x61, 0x6c, 0x69, 0x7a, 0x65, 0x64, 0x12,
	0x25, 0x0a, 0x0e, 0x6e, 0x6f, 0x6e, 0x5f, 0x6e, 0x6f, 0x72, 0x6d, 0x61, 0x6c, 0x69, 0x7a, 0x65,
	0x64, 0x18, 0x03, 0x20, 0x01, 0x28, 0x02, 0x52, 0x0d, 0x6e, 0x6f, 0x6e, 0x4e, 0x6f, 0x72, 0x6d,
	0x61, 0x6c, 0x69, 0x7a, 0x65, 0x64, 0x22, 0x53, 0x0a, 0x11, 0x42, 0x6f, 0x6f, 0x74, 0x73, 0x74,
	0x72, 0x61, 0x70, 0x41, 0x64, 0x64, 0x72, 0x73, 0x52, 0x65, 0x71, 0x12, 0x1b, 0x0a, 0x09, 0x6e,
	0x75, 0x6d, 0x5f, 0x61, 0x64, 0x64, 0x72, 0x73, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x08,
	0x6e, 0x75, 0x6d, 0x41, 0x64, 0x64, 0x72, 0x73, 0x12, 0x21, 0x0a, 0x0c, 0x69, 0x67, 0x6e, 0x6f,
	0x72, 0x65, 0x5f, 0x6e, 0x6f, 0x64, 0x65, 0x73, 0x18, 0x02, 0x20, 0x03, 0x28, 0x0c, 0x52, 0x0b,
	0x69, 0x67, 0x6e, 0x6f, 0x72, 0x65, 0x4e, 0x6f, 0x64, 0x65, 0x73, 0x22, 0x40, 0x0a, 0x12, 0x42,
	0x6f, 0x6f, 0x74, 0x73, 0x74, 0x72, 0x61, 0x70, 0x41, 0x64, 0x64, 0x72, 0x73, 0x52, 0x65, 0x73,
	0x70, 0x12, 0x2a, 0x0a, 0x05, 0x61, 0x64, 0x64, 0x72, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b,
	0x32, 0x14, 0x2e, 0x67, 0x72, 0x61, 0x70, 0x68, 0x72, 0x70, 0x63, 0x2e, 0x4e, 0x65, 0x74, 0x41,
	0x64, 0x64, 0x72, 0x65, 0x73, 0x73, 0x52, 0x05, 0x61, 0x64, 0x64, 0x72, 0x73, 0x22, 0x53, 0x0a,
	0x0a, 0x4e, 0x65, 0x74, 0x41, 0x64, 0x64, 0x72, 0x65, 0x73, 0x73, 0x12, 0x17, 0x0a, 0x07, 0x6e,
	0x6f, 0x64, 0x65, 0x5f, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x06, 0x6e, 0x6f,
	0x64, 0x65, 0x49, 0x64, 0x12, 0x2c, 0x0a, 0x07, 0x61, 0x64, 0x64, 0x72, 0x65, 0x73, 0x73, 0x18,
	0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x12, 0x2e, 0x6c, 0x6e, 0x72, 0x70, 0x63, 0x2e, 0x4e, 0x6f,
	0x64, 0x65, 0x41, 0x64, 0x64, 0x72, 0x65, 0x73, 0x73, 0x52, 0x07, 0x61, 0x64, 0x64, 0x72, 0x65,
	0x73, 0x73, 0x22, 0x14, 0x0a, 0x12, 0x42, 0x6f, 0x6f, 0x73, 0x74, 0x72, 0x61, 0x70, 0x70, 0x65,
	0x72, 0x4e, 0x61, 0x6d, 0x65, 0x52, 0x65, 0x71, 0x22, 0x29, 0x0a, 0x13, 0x42, 0x6f, 0x6f, 0x73,
	0x74, 0x72, 0x61, 0x70, 0x70, 0x65, 0x72, 0x4e, 0x61, 0x6d, 0x65, 0x52, 0x65, 0x73, 0x70, 0x12,
	0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e,
	0x61, 0x6d, 0x65, 0x32, 0x86, 0x02, 0x0a, 0x05, 0x47, 0x72, 0x61, 0x70, 0x68, 0x12, 0x4b, 0x0a,
	0x0e, 0x42, 0x6f, 0x6f, 0x74, 0x73, 0x74, 0x72, 0x61, 0x70, 0x41, 0x64, 0x64, 0x72, 0x73, 0x12,
	0x1b, 0x2e, 0x67, 0x72, 0x61, 0x70, 0x68, 0x72, 0x70, 0x63, 0x2e, 0x42, 0x6f, 0x6f, 0x74, 0x73,
	0x74, 0x72, 0x61, 0x70, 0x41, 0x64, 0x64, 0x72, 0x73, 0x52, 0x65, 0x71, 0x1a, 0x1c, 0x2e, 0x67,
	0x72, 0x61, 0x70, 0x68, 0x72, 0x70, 0x63, 0x2e, 0x42, 0x6f, 0x6f, 0x74, 0x73, 0x74, 0x72, 0x61,
	0x70, 0x41, 0x64, 0x64, 0x72, 0x73, 0x52, 0x65, 0x73, 0x70, 0x12, 0x4e, 0x0a, 0x0f, 0x42, 0x6f,
	0x6f, 0x73, 0x74, 0x72, 0x61, 0x70, 0x70, 0x65, 0x72, 0x4e, 0x61, 0x6d, 0x65, 0x12, 0x1c, 0x2e,
	0x67, 0x72, 0x61, 0x70, 0x68, 0x72, 0x70, 0x63, 0x2e, 0x42, 0x6f, 0x6f, 0x73, 0x74, 0x72, 0x61,
	0x70, 0x70, 0x65, 0x72, 0x4e, 0x61, 0x6d, 0x65, 0x52, 0x65, 0x71, 0x1a, 0x1d, 0x2e, 0x67, 0x72,
	0x61, 0x70, 0x68, 0x72, 0x70, 0x63, 0x2e, 0x42, 0x6f, 0x6f, 0x73, 0x74, 0x72, 0x61, 0x70, 0x70,
	0x65, 0x72, 0x4e, 0x61, 0x6d, 0x65, 0x52, 0x65, 0x73, 0x70, 0x12, 0x60, 0x0a, 0x15, 0x42, 0x65,
	0x74, 0x77, 0x65, 0x65, 0x6e, 0x6e, 0x65, 0x73, 0x73, 0x43, 0x65, 0x6e, 0x74, 0x72, 0x61, 0x6c,
	0x69, 0x74, 0x79, 0x12, 0x22, 0x2e, 0x67, 0x72, 0x61, 0x70, 0x68, 0x72, 0x70, 0x63, 0x2e, 0x42,
	0x65, 0x74, 0x77, 0x65, 0x65, 0x6e, 0x6e, 0x65, 0x73, 0x73, 0x43, 0x65, 0x6e, 0x74, 0x72, 0x61,
	0x6c, 0x69, 0x74, 0x79, 0x52, 0x65, 0x71, 0x1a, 0x23, 0x2e, 0x67, 0x72, 0x61, 0x70, 0x68, 0x72,
	0x70, 0x63, 0x2e, 0x42, 0x65, 0x74, 0x77, 0x65, 0x65, 0x6e, 0x6e, 0x65, 0x73, 0x73, 0x43, 0x65,
	0x6e, 0x74, 0x72, 0x61, 0x6c, 0x69, 0x74, 0x79, 0x52, 0x65, 0x73, 0x70, 0x42, 0x30, 0x5a, 0x2e,
	0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x6c, 0x69, 0x67, 0x68, 0x74,
	0x6e, 0x69, 0x6e, 0x67, 0x6e, 0x65, 0x74, 0x77, 0x6f, 0x72, 0x6b, 0x2f, 0x6c, 0x6e, 0x64, 0x2f,
	0x6c, 0x6e, 0x72, 0x70, 0x63, 0x2f, 0x67, 0x72, 0x61, 0x70, 0x68, 0x72, 0x70, 0x63, 0x62, 0x06,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_graphrpc_graph_proto_rawDescOnce sync.Once
	file_graphrpc_graph_proto_rawDescData = file_graphrpc_graph_proto_rawDesc
)

func file_graphrpc_graph_proto_rawDescGZIP() []byte {
	file_graphrpc_graph_proto_rawDescOnce.Do(func() {
		file_graphrpc_graph_proto_rawDescData = protoimpl.X.CompressGZIP(file_graphrpc_graph_proto_rawDescData)
	})
	return file_graphrpc_graph_proto_rawDescData
}

var file_graphrpc_graph_proto_msgTypes = make([]protoimpl.MessageInfo, 8)
var file_graphrpc_graph_proto_goTypes = []interface{}{
	(*BetweennessCentralityReq)(nil),  // 0: graphrpc.BetweennessCentralityReq
	(*BetweennessCentralityResp)(nil), // 1: graphrpc.BetweennessCentralityResp
	(*BetweenessCentrality)(nil),      // 2: graphrpc.BetweenessCentrality
	(*BootstrapAddrsReq)(nil),         // 3: graphrpc.BootstrapAddrsReq
	(*BootstrapAddrsResp)(nil),        // 4: graphrpc.BootstrapAddrsResp
	(*NetAddress)(nil),                // 5: graphrpc.NetAddress
	(*BoostrapperNameReq)(nil),        // 6: graphrpc.BoostrapperNameReq
	(*BoostrapperNameResp)(nil),       // 7: graphrpc.BoostrapperNameResp
	(*lnrpc.NodeAddress)(nil),         // 8: lnrpc.NodeAddress
}
var file_graphrpc_graph_proto_depIdxs = []int32{
	2, // 0: graphrpc.BetweennessCentralityResp.node_betweeness:type_name -> graphrpc.BetweenessCentrality
	5, // 1: graphrpc.BootstrapAddrsResp.addrs:type_name -> graphrpc.NetAddress
	8, // 2: graphrpc.NetAddress.address:type_name -> lnrpc.NodeAddress
	3, // 3: graphrpc.Graph.BootstrapAddrs:input_type -> graphrpc.BootstrapAddrsReq
	6, // 4: graphrpc.Graph.BoostrapperName:input_type -> graphrpc.BoostrapperNameReq
	0, // 5: graphrpc.Graph.BetweennessCentrality:input_type -> graphrpc.BetweennessCentralityReq
	4, // 6: graphrpc.Graph.BootstrapAddrs:output_type -> graphrpc.BootstrapAddrsResp
	7, // 7: graphrpc.Graph.BoostrapperName:output_type -> graphrpc.BoostrapperNameResp
	1, // 8: graphrpc.Graph.BetweennessCentrality:output_type -> graphrpc.BetweennessCentralityResp
	6, // [6:9] is the sub-list for method output_type
	3, // [3:6] is the sub-list for method input_type
	3, // [3:3] is the sub-list for extension type_name
	3, // [3:3] is the sub-list for extension extendee
	0, // [0:3] is the sub-list for field type_name
}

func init() { file_graphrpc_graph_proto_init() }
func file_graphrpc_graph_proto_init() {
	if File_graphrpc_graph_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_graphrpc_graph_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*BetweennessCentralityReq); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_graphrpc_graph_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*BetweennessCentralityResp); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_graphrpc_graph_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*BetweenessCentrality); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_graphrpc_graph_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*BootstrapAddrsReq); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_graphrpc_graph_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*BootstrapAddrsResp); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_graphrpc_graph_proto_msgTypes[5].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*NetAddress); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_graphrpc_graph_proto_msgTypes[6].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*BoostrapperNameReq); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_graphrpc_graph_proto_msgTypes[7].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*BoostrapperNameResp); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_graphrpc_graph_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   8,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_graphrpc_graph_proto_goTypes,
		DependencyIndexes: file_graphrpc_graph_proto_depIdxs,
		MessageInfos:      file_graphrpc_graph_proto_msgTypes,
	}.Build()
	File_graphrpc_graph_proto = out.File
	file_graphrpc_graph_proto_rawDesc = nil
	file_graphrpc_graph_proto_goTypes = nil
	file_graphrpc_graph_proto_depIdxs = nil
}
