package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_1/grpc_gateway_swagger_validation/pkg/proto/ufo/v1"
)

// randomString генерирует строку заданной длины для тестов
func randomString(length int) string {
	return strings.Repeat("A", length)
}

// ============ Location Validation Tests ============.

func TestValidation_LocationTooShort(t *testing.T) {
	ctx := context.Background()

	_, err := client.Create(ctx, &ufov1.CreateRequest{
		Info: &ufov1.SightingInfo{
			ObservedAt:  timestamppb.New(time.Now()),
			Location:    "",
			Description: "Description",
		},
	})

	assertGRPCStatus(t, err, codes.InvalidArgument)
}

func TestValidation_LocationTooLong(t *testing.T) {
	ctx := context.Background()

	longLocation := randomString(51)

	_, err := client.Create(ctx, &ufov1.CreateRequest{
		Info: &ufov1.SightingInfo{
			ObservedAt:  timestamppb.New(time.Now()),
			Location:    longLocation,
			Description: "Description",
		},
	})

	assertGRPCStatus(t, err, codes.InvalidArgument)
}

func TestValidation_LocationMinBoundary(t *testing.T) {
	ctx := context.Background()

	resp, err := client.Create(ctx, &ufov1.CreateRequest{
		Info: &ufov1.SightingInfo{
			ObservedAt:  timestamppb.New(time.Now()),
			Location:    "A",
			Description: "Description",
		},
	})

	require.NoError(t, err, "location из 1 символа должен быть валидным")
	require.NotEmpty(t, resp.GetUuid())

	getResp, err := client.Get(ctx, &ufov1.GetRequest{Uuid: resp.GetUuid()})
	require.NoError(t, err)
	require.Equal(t, "A", getResp.GetSighting().GetInfo().GetLocation())
}

func TestValidation_LocationMaxBoundary(t *testing.T) {
	ctx := context.Background()

	maxLocation := randomString(50)

	resp, err := client.Create(ctx, &ufov1.CreateRequest{
		Info: &ufov1.SightingInfo{
			ObservedAt:  timestamppb.New(time.Now()),
			Location:    maxLocation,
			Description: "Description",
		},
	})

	require.NoError(t, err, "location из 50 символов должен быть валидным")
	require.NotEmpty(t, resp.GetUuid())

	getResp, err := client.Get(ctx, &ufov1.GetRequest{Uuid: resp.GetUuid()})
	require.NoError(t, err)
	require.Equal(t, maxLocation, getResp.GetSighting().GetInfo().GetLocation())
}

func TestValidation_NilInfoField(t *testing.T) {
	ctx := context.Background()

	_, err := client.Create(ctx, &ufov1.CreateRequest{
		Info: nil,
	})

	assertGRPCStatus(t, err, codes.InvalidArgument)
}

func TestValidation_EmptyRequest(t *testing.T) {
	ctx := context.Background()

	_, err := client.Create(ctx, &ufov1.CreateRequest{})

	assertGRPCStatus(t, err, codes.InvalidArgument)
}
