// Code generated by protoc-gen-go-grpc. DO NOT EDIT.

package graphrpc

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// GraphClient is the client API for Graph service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type GraphClient interface {
	BootstrapAddrs(ctx context.Context, in *BootstrapAddrsReq, opts ...grpc.CallOption) (*BootstrapAddrsResp, error)
	BoostrapperName(ctx context.Context, in *BoostrapperNameReq, opts ...grpc.CallOption) (*BoostrapperNameResp, error)
}

type graphClient struct {
	cc grpc.ClientConnInterface
}

func NewGraphClient(cc grpc.ClientConnInterface) GraphClient {
	return &graphClient{cc}
}

func (c *graphClient) BootstrapAddrs(ctx context.Context, in *BootstrapAddrsReq, opts ...grpc.CallOption) (*BootstrapAddrsResp, error) {
	out := new(BootstrapAddrsResp)
	err := c.cc.Invoke(ctx, "/graphrpc.Graph/BootstrapAddrs", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *graphClient) BoostrapperName(ctx context.Context, in *BoostrapperNameReq, opts ...grpc.CallOption) (*BoostrapperNameResp, error) {
	out := new(BoostrapperNameResp)
	err := c.cc.Invoke(ctx, "/graphrpc.Graph/BoostrapperName", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// GraphServer is the server API for Graph service.
// All implementations must embed UnimplementedGraphServer
// for forward compatibility
type GraphServer interface {
	BootstrapAddrs(context.Context, *BootstrapAddrsReq) (*BootstrapAddrsResp, error)
	BoostrapperName(context.Context, *BoostrapperNameReq) (*BoostrapperNameResp, error)
	mustEmbedUnimplementedGraphServer()
}

// UnimplementedGraphServer must be embedded to have forward compatible implementations.
type UnimplementedGraphServer struct {
}

func (UnimplementedGraphServer) BootstrapAddrs(context.Context, *BootstrapAddrsReq) (*BootstrapAddrsResp, error) {
	return nil, status.Errorf(codes.Unimplemented, "method BootstrapAddrs not implemented")
}
func (UnimplementedGraphServer) BoostrapperName(context.Context, *BoostrapperNameReq) (*BoostrapperNameResp, error) {
	return nil, status.Errorf(codes.Unimplemented, "method BoostrapperName not implemented")
}
func (UnimplementedGraphServer) mustEmbedUnimplementedGraphServer() {}

// UnsafeGraphServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to GraphServer will
// result in compilation errors.
type UnsafeGraphServer interface {
	mustEmbedUnimplementedGraphServer()
}

func RegisterGraphServer(s grpc.ServiceRegistrar, srv GraphServer) {
	s.RegisterService(&Graph_ServiceDesc, srv)
}

func _Graph_BootstrapAddrs_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(BootstrapAddrsReq)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(GraphServer).BootstrapAddrs(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/graphrpc.Graph/BootstrapAddrs",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(GraphServer).BootstrapAddrs(ctx, req.(*BootstrapAddrsReq))
	}
	return interceptor(ctx, in, info, handler)
}

func _Graph_BoostrapperName_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(BoostrapperNameReq)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(GraphServer).BoostrapperName(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/graphrpc.Graph/BoostrapperName",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(GraphServer).BoostrapperName(ctx, req.(*BoostrapperNameReq))
	}
	return interceptor(ctx, in, info, handler)
}

// Graph_ServiceDesc is the grpc.ServiceDesc for Graph service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Graph_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "graphrpc.Graph",
	HandlerType: (*GraphServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "BootstrapAddrs",
			Handler:    _Graph_BootstrapAddrs_Handler,
		},
		{
			MethodName: "BoostrapperName",
			Handler:    _Graph_BoostrapperName_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "graphrpc/graph.proto",
}
