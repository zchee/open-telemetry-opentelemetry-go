// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"log"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/example/namedtracer/foo"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

var (
	fooKey     = attribute.Key("ex.com/foo")
	barKey     = attribute.Key("ex.com/bar")
	anotherKey = attribute.Key("ex.com/another")
)

var tp *sdktrace.TracerProvider

// initTracer creates and registers trace provider instance.
func initTracer() {
	var err error
	exp, err := stdouttrace.New(
		stdouttrace.WithPrettyPrint(),
	)
	if err != nil {
		log.Panicf("failed to initialize stdouttrace exporter %v\n", err)
		return
	}

	res, _ := resource.New(context.Background(),
		resource.WithSchemaURL(semconv.SchemaURL),
		resource.WithTelemetrySDK(),
		resource.WithFromEnv(),
	)
	bsp := sdktrace.NewBatchSpanProcessor(exp)
	tp = sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(bsp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
}

func main() {
	// initialize trace provider.
	initTracer()

	// Create a named tracer with package path as its name.
	tracer := tp.Tracer("example/namedtracer/main",
		trace.WithInstrumentationVersion("0.0.0"),
		trace.WithSchemaURL(semconv.SchemaURL),
	)
	ctx := context.Background()
	defer func() { _ = tp.Shutdown(ctx) }()

	m0, _ := baggage.NewMember(string(fooKey), "foo1")
	m1, _ := baggage.NewMember(string(barKey), "bar1")
	b, _ := baggage.New(m0, m1)
	ctx = baggage.ContextWithBaggage(ctx, b)

	var span trace.Span
	ctx, span = tracer.Start(ctx, "operation")
	defer span.End()

	span.AddEvent("Nice operation!", trace.WithAttributes(attribute.Int("bogons", 100)))
	span.SetAttributes(anotherKey.String("yes"))

	if err := MainOperation(ctx); err != nil {
		panic(err)
	}

	if err := foo.SubOperation(ctx); err != nil {
		panic(err)
	}
}

var (
	orangeKey = attribute.Key("ex.com/oranges")
)

// MainOperation is an example to demonstrate the use of named tracer.
// It creates a named tracer with same package path.
func MainOperation(ctx context.Context) error {
	// Using global provider. Alternative is to have application provide a getter
	// for its component to get the instance of the provider.
	tr := otel.Tracer("example/namedtracer/main")

	ctx, span := tr.Start(ctx, "Main operation...")
	defer span.End()

	span.SetAttributes(orangeKey.String("four"))
	span.AddEvent("Main span event")

	return nil
}

// Output:
// [
// 	{
// 		"Name": "Main operation...",
// 		"SpanContext": {
// 			"TraceID": "82e1a5ef0835a977a7b027192c958f72",
// 			"SpanID": "6197082d49ef3eae",
// 			"TraceFlags": "01",
// 			"TraceState": "",
// 			"Remote": false
// 		},
// 		"Parent": {
// 			"TraceID": "82e1a5ef0835a977a7b027192c958f72",
// 			"SpanID": "c3a8114e8186c933",
// 			"TraceFlags": "01",
// 			"TraceState": "",
// 			"Remote": false
// 		},
// 		"SpanKind": 1,
// 		"StartTime": "2021-07-20T07:47:45.247955+09:00",
// 		"EndTime": "2021-07-20T07:47:45.24795584+09:00",
// 		"Attributes": [
// 			{
// 				"Key": "ex.com/oranges",
// 				"Value": {
// 					"Type": "STRING",
// 					"Value": "four"
// 				}
// 			}
// 		],
// 		"Events": [
// 			{
// 				"Name": "Main span event",
// 				"Attributes": null,
// 				"DroppedAttributeCount": 0,
// 				"Time": "2021-07-20T07:47:45.247956+09:00"
// 			}
// 		],
// 		"Links": null,
// 		"Status": {
// 			"Code": "Unset",
// 			"Description": ""
// 		},
// 		"DroppedAttributes": 0,
// 		"DroppedEvents": 0,
// 		"DroppedLinks": 0,
// 		"ChildSpanCount": 0,
// 		"Resource": [
// 			{
// 				"Key": "telemetry.sdk.language",
// 				"Value": {
// 					"Type": "STRING",
// 					"Value": "go"
// 				}
// 			},
// 			{
// 				"Key": "telemetry.sdk.name",
// 				"Value": {
// 					"Type": "STRING",
// 					"Value": "opentelemetry"
// 				}
// 			},
// 			{
// 				"Key": "telemetry.sdk.version",
// 				"Value": {
// 					"Type": "STRING",
// 					"Value": "1.0.0-RC1"
// 				}
// 			}
// 		],
// 		"InstrumentationLibrary": {
// 			"Name": "example/namedtracer/main",
// 			"Version": "",
// 			"SchemaURL": ""
// 		}
// 	},
// 	{
// 		"Name": "Sub operation...",
// 		"SpanContext": {
// 			"TraceID": "82e1a5ef0835a977a7b027192c958f72",
// 			"SpanID": "8abb71052e19cfff",
// 			"TraceFlags": "01",
// 			"TraceState": "",
// 			"Remote": false
// 		},
// 		"Parent": {
// 			"TraceID": "82e1a5ef0835a977a7b027192c958f72",
// 			"SpanID": "c3a8114e8186c933",
// 			"TraceFlags": "01",
// 			"TraceState": "",
// 			"Remote": false
// 		},
// 		"SpanKind": 1,
// 		"StartTime": "2021-07-20T07:47:45.247965+09:00",
// 		"EndTime": "2021-07-20T07:47:45.247965706+09:00",
// 		"Attributes": [
// 			{
// 				"Key": "ex.com/lemons",
// 				"Value": {
// 					"Type": "STRING",
// 					"Value": "five"
// 				}
// 			}
// 		],
// 		"Events": [
// 			{
// 				"Name": "Sub span event",
// 				"Attributes": null,
// 				"DroppedAttributeCount": 0,
// 				"Time": "2021-07-20T07:47:45.247965+09:00"
// 			}
// 		],
// 		"Links": null,
// 		"Status": {
// 			"Code": "Unset",
// 			"Description": ""
// 		},
// 		"DroppedAttributes": 0,
// 		"DroppedEvents": 0,
// 		"DroppedLinks": 0,
// 		"ChildSpanCount": 0,
// 		"Resource": [
// 			{
// 				"Key": "telemetry.sdk.language",
// 				"Value": {
// 					"Type": "STRING",
// 					"Value": "go"
// 				}
// 			},
// 			{
// 				"Key": "telemetry.sdk.name",
// 				"Value": {
// 					"Type": "STRING",
// 					"Value": "opentelemetry"
// 				}
// 			},
// 			{
// 				"Key": "telemetry.sdk.version",
// 				"Value": {
// 					"Type": "STRING",
// 					"Value": "1.0.0-RC1"
// 				}
// 			}
// 		],
// 		"InstrumentationLibrary": {
// 			"Name": "example/namedtracer/foo",
// 			"Version": "",
// 			"SchemaURL": ""
// 		}
// 	},
// 	{
// 		"Name": "operation",
// 		"SpanContext": {
// 			"TraceID": "82e1a5ef0835a977a7b027192c958f72",
// 			"SpanID": "c3a8114e8186c933",
// 			"TraceFlags": "01",
// 			"TraceState": "",
// 			"Remote": false
// 		},
// 		"Parent": {
// 			"TraceID": "00000000000000000000000000000000",
// 			"SpanID": "0000000000000000",
// 			"TraceFlags": "00",
// 			"TraceState": "",
// 			"Remote": false
// 		},
// 		"SpanKind": 1,
// 		"StartTime": "2021-07-20T07:47:45.247945+09:00",
// 		"EndTime": "2021-07-20T07:47:45.247967529+09:00",
// 		"Attributes": [
// 			{
// 				"Key": "ex.com/another",
// 				"Value": {
// 					"Type": "STRING",
// 					"Value": "yes"
// 				}
// 			}
// 		],
// 		"Events": [
// 			{
// 				"Name": "Nice operation!",
// 				"Attributes": [
// 					{
// 						"Key": "bogons",
// 						"Value": {
// 							"Type": "INT64",
// 							"Value": 100
// 						}
// 					}
// 				],
// 				"DroppedAttributeCount": 0,
// 				"Time": "2021-07-20T07:47:45.24795+09:00"
// 			}
// 		],
// 		"Links": null,
// 		"Status": {
// 			"Code": "Unset",
// 			"Description": ""
// 		},
// 		"DroppedAttributes": 0,
// 		"DroppedEvents": 0,
// 		"DroppedLinks": 0,
// 		"ChildSpanCount": 2,
// 		"Resource": [
// 			{
// 				"Key": "telemetry.sdk.language",
// 				"Value": {
// 					"Type": "STRING",
// 					"Value": "go"
// 				}
// 			},
// 			{
// 				"Key": "telemetry.sdk.name",
// 				"Value": {
// 					"Type": "STRING",
// 					"Value": "opentelemetry"
// 				}
// 			},
// 			{
// 				"Key": "telemetry.sdk.version",
// 				"Value": {
// 					"Type": "STRING",
// 					"Value": "1.0.0-RC1"
// 				}
// 			}
// 		],
// 		"InstrumentationLibrary": {
// 			"Name": "example/namedtracer/main",
// 			"Version": "0.0.0",
// 			"SchemaURL": "https://opentelemetry.io/schemas/v1.4.0"
// 		}
// 	}
// ]
