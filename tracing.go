package namesys

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// Deprecated: use github.com/ipfs/boxo/namesys.StartSpan
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return otel.Tracer("go-namesys").Start(ctx, fmt.Sprintf("Namesys.%s", name))
}
