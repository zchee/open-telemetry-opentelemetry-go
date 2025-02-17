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

package metric_test

import (
	"context"
	"fmt"
	"math"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/sdkapi"
	export "go.opentelemetry.io/otel/sdk/export/metric"
	"go.opentelemetry.io/otel/sdk/export/metric/aggregation"
	metricsdk "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/processor/processortest"
)

var Must = metric.Must

type handler struct {
	sync.Mutex
	err error
}

func (h *handler) Handle(err error) {
	h.Lock()
	h.err = err
	h.Unlock()
}

func (h *handler) Reset() {
	h.Lock()
	h.err = nil
	h.Unlock()
}

func (h *handler) Flush() error {
	h.Lock()
	err := h.err
	h.err = nil
	h.Unlock()
	return err
}

var testHandler *handler

func init() {
	testHandler = new(handler)
	otel.SetErrorHandler(testHandler)
}

type testSelector struct {
	selector    export.AggregatorSelector
	newAggCount int
}

func (ts *testSelector) AggregatorFor(desc *sdkapi.Descriptor, aggPtrs ...*export.Aggregator) {
	ts.newAggCount += len(aggPtrs)
	processortest.AggregatorSelector().AggregatorFor(desc, aggPtrs...)
}

func newSDK(t *testing.T) (metric.Meter, *metricsdk.Accumulator, *testSelector, *processortest.Processor) {
	testHandler.Reset()
	testSelector := &testSelector{selector: processortest.AggregatorSelector()}
	processor := processortest.NewProcessor(
		testSelector,
		attribute.DefaultEncoder(),
	)
	accum := metricsdk.NewAccumulator(
		processor,
	)
	meter := metric.WrapMeterImpl(accum)
	return meter, accum, testSelector, processor
}

func TestInputRangeCounter(t *testing.T) {
	ctx := context.Background()
	meter, sdk, _, processor := newSDK(t)

	counter := Must(meter).NewInt64Counter("name.sum")

	counter.Add(ctx, -1)
	require.Equal(t, aggregation.ErrNegativeInput, testHandler.Flush())

	checkpointed := sdk.Collect(ctx)
	require.Equal(t, 0, checkpointed)

	processor.Reset()
	counter.Add(ctx, 1)
	checkpointed = sdk.Collect(ctx)
	require.Equal(t, map[string]float64{
		"name.sum//": 1,
	}, processor.Values())
	require.Equal(t, 1, checkpointed)
	require.Nil(t, testHandler.Flush())
}

func TestInputRangeUpDownCounter(t *testing.T) {
	ctx := context.Background()
	meter, sdk, _, processor := newSDK(t)

	counter := Must(meter).NewInt64UpDownCounter("name.sum")

	counter.Add(ctx, -1)
	counter.Add(ctx, -1)
	counter.Add(ctx, 2)
	counter.Add(ctx, 1)

	checkpointed := sdk.Collect(ctx)
	require.Equal(t, map[string]float64{
		"name.sum//": 1,
	}, processor.Values())
	require.Equal(t, 1, checkpointed)
	require.Nil(t, testHandler.Flush())
}

func TestInputRangeHistogram(t *testing.T) {
	ctx := context.Background()
	meter, sdk, _, processor := newSDK(t)

	histogram := Must(meter).NewFloat64Histogram("name.histogram")

	histogram.Record(ctx, math.NaN())
	require.Equal(t, aggregation.ErrNaNInput, testHandler.Flush())

	checkpointed := sdk.Collect(ctx)
	require.Equal(t, 0, checkpointed)

	histogram.Record(ctx, 1)
	histogram.Record(ctx, 2)

	processor.Reset()
	checkpointed = sdk.Collect(ctx)

	require.Equal(t, map[string]float64{
		"name.histogram//": 3,
	}, processor.Values())
	require.Equal(t, 1, checkpointed)
	require.Nil(t, testHandler.Flush())
}

func TestDisabledInstrument(t *testing.T) {
	ctx := context.Background()
	meter, sdk, _, processor := newSDK(t)

	histogram := Must(meter).NewFloat64Histogram("name.disabled")

	histogram.Record(ctx, -1)
	checkpointed := sdk.Collect(ctx)

	require.Equal(t, 0, checkpointed)
	require.Equal(t, map[string]float64{}, processor.Values())
}

func TestRecordNaN(t *testing.T) {
	ctx := context.Background()
	meter, _, _, _ := newSDK(t)

	c := Must(meter).NewFloat64Counter("name.sum")

	require.Nil(t, testHandler.Flush())
	c.Add(ctx, math.NaN())
	require.Error(t, testHandler.Flush())
}

func TestSDKLabelsDeduplication(t *testing.T) {
	ctx := context.Background()
	meter, sdk, _, processor := newSDK(t)

	counter := Must(meter).NewInt64Counter("name.sum")

	const (
		maxKeys = 21
		keySets = 2
		repeats = 3
	)
	var keysA []attribute.Key
	var keysB []attribute.Key

	for i := 0; i < maxKeys; i++ {
		keysA = append(keysA, attribute.Key(fmt.Sprintf("A%03d", i)))
		keysB = append(keysB, attribute.Key(fmt.Sprintf("B%03d", i)))
	}

	allExpect := map[string]float64{}
	for numKeys := 0; numKeys < maxKeys; numKeys++ {

		var kvsA []attribute.KeyValue
		var kvsB []attribute.KeyValue
		for r := 0; r < repeats; r++ {
			for i := 0; i < numKeys; i++ {
				kvsA = append(kvsA, keysA[i].Int(r))
				kvsB = append(kvsB, keysB[i].Int(r))
			}
		}

		var expectA []attribute.KeyValue
		var expectB []attribute.KeyValue
		for i := 0; i < numKeys; i++ {
			expectA = append(expectA, keysA[i].Int(repeats-1))
			expectB = append(expectB, keysB[i].Int(repeats-1))
		}

		counter.Add(ctx, 1, kvsA...)
		counter.Add(ctx, 1, kvsA...)
		format := func(attrs []attribute.KeyValue) string {
			str := attribute.DefaultEncoder().Encode(newSetIter(attrs...))
			return fmt.Sprint("name.sum/", str, "/")
		}
		allExpect[format(expectA)] += 2

		if numKeys != 0 {
			// In this case A and B sets are the same.
			counter.Add(ctx, 1, kvsB...)
			counter.Add(ctx, 1, kvsB...)
			allExpect[format(expectB)] += 2
		}

	}

	sdk.Collect(ctx)

	require.EqualValues(t, allExpect, processor.Values())
}

func newSetIter(kvs ...attribute.KeyValue) attribute.Iterator {
	labels := attribute.NewSet(kvs...)
	return labels.Iter()
}

func TestDefaultLabelEncoder(t *testing.T) {
	encoder := attribute.DefaultEncoder()

	encoded := encoder.Encode(newSetIter(attribute.String("A", "B"), attribute.String("C", "D")))
	require.Equal(t, `A=B,C=D`, encoded)

	encoded = encoder.Encode(newSetIter(attribute.String("A", "B,c=d"), attribute.String(`C\`, "D")))
	require.Equal(t, `A=B\,c\=d,C\\=D`, encoded)

	encoded = encoder.Encode(newSetIter(attribute.String(`\`, `=`), attribute.String(`,`, `\`)))
	require.Equal(t, `\,=\\,\\=\=`, encoded)

	// Note: the label encoder does not sort or de-dup values,
	// that is done in Labels(...).
	encoded = encoder.Encode(newSetIter(
		attribute.Int("I", 1),
		attribute.Int64("I64", 1),
		attribute.Float64("F64", 1),
		attribute.Float64("F64", 1),
		attribute.String("S", "1"),
		attribute.Bool("B", true),
	))
	require.Equal(t, "B=true,F64=1,I=1,I64=1,S=1", encoded)
}

func TestObserverCollection(t *testing.T) {
	ctx := context.Background()
	meter, sdk, _, processor := newSDK(t)
	mult := 1

	_ = Must(meter).NewFloat64GaugeObserver("float.gauge.lastvalue", func(_ context.Context, result metric.Float64ObserverResult) {
		result.Observe(float64(mult), attribute.String("A", "B"))
		// last value wins
		result.Observe(float64(-mult), attribute.String("A", "B"))
		result.Observe(float64(-mult), attribute.String("C", "D"))
	})
	_ = Must(meter).NewInt64GaugeObserver("int.gauge.lastvalue", func(_ context.Context, result metric.Int64ObserverResult) {
		result.Observe(int64(-mult), attribute.String("A", "B"))
		result.Observe(int64(mult))
		// last value wins
		result.Observe(int64(mult), attribute.String("A", "B"))
		result.Observe(int64(mult))
	})

	_ = Must(meter).NewFloat64CounterObserver("float.counterobserver.sum", func(_ context.Context, result metric.Float64ObserverResult) {
		result.Observe(float64(mult), attribute.String("A", "B"))
		result.Observe(float64(2*mult), attribute.String("A", "B"))
		result.Observe(float64(mult), attribute.String("C", "D"))
	})
	_ = Must(meter).NewInt64CounterObserver("int.counterobserver.sum", func(_ context.Context, result metric.Int64ObserverResult) {
		result.Observe(int64(2*mult), attribute.String("A", "B"))
		result.Observe(int64(mult))
		// last value wins
		result.Observe(int64(mult), attribute.String("A", "B"))
		result.Observe(int64(mult))
	})

	_ = Must(meter).NewFloat64UpDownCounterObserver("float.updowncounterobserver.sum", func(_ context.Context, result metric.Float64ObserverResult) {
		result.Observe(float64(mult), attribute.String("A", "B"))
		result.Observe(float64(-2*mult), attribute.String("A", "B"))
		result.Observe(float64(mult), attribute.String("C", "D"))
	})
	_ = Must(meter).NewInt64UpDownCounterObserver("int.updowncounterobserver.sum", func(_ context.Context, result metric.Int64ObserverResult) {
		result.Observe(int64(2*mult), attribute.String("A", "B"))
		result.Observe(int64(mult))
		// last value wins
		result.Observe(int64(mult), attribute.String("A", "B"))
		result.Observe(int64(-mult))
	})

	_ = Must(meter).NewInt64GaugeObserver("empty.gauge.sum", func(_ context.Context, result metric.Int64ObserverResult) {
	})

	for mult = 0; mult < 3; mult++ {
		processor.Reset()

		collected := sdk.Collect(ctx)
		require.Equal(t, collected, len(processor.Values()))

		mult := float64(mult)
		require.EqualValues(t, map[string]float64{
			"float.gauge.lastvalue/A=B/": -mult,
			"float.gauge.lastvalue/C=D/": -mult,
			"int.gauge.lastvalue//":      mult,
			"int.gauge.lastvalue/A=B/":   mult,

			"float.counterobserver.sum/A=B/": 2 * mult,
			"float.counterobserver.sum/C=D/": mult,
			"int.counterobserver.sum//":      mult,
			"int.counterobserver.sum/A=B/":   mult,

			"float.updowncounterobserver.sum/A=B/": -2 * mult,
			"float.updowncounterobserver.sum/C=D/": mult,
			"int.updowncounterobserver.sum//":      -mult,
			"int.updowncounterobserver.sum/A=B/":   mult,
		}, processor.Values())
	}
}

func TestCounterObserverInputRange(t *testing.T) {
	ctx := context.Background()
	meter, sdk, _, processor := newSDK(t)

	// TODO: these tests are testing for negative values, not for _descending values_. Fix.
	_ = Must(meter).NewFloat64CounterObserver("float.counterobserver.sum", func(_ context.Context, result metric.Float64ObserverResult) {
		result.Observe(-2, attribute.String("A", "B"))
		require.Equal(t, aggregation.ErrNegativeInput, testHandler.Flush())
		result.Observe(-1, attribute.String("C", "D"))
		require.Equal(t, aggregation.ErrNegativeInput, testHandler.Flush())
	})
	_ = Must(meter).NewInt64CounterObserver("int.counterobserver.sum", func(_ context.Context, result metric.Int64ObserverResult) {
		result.Observe(-1, attribute.String("A", "B"))
		require.Equal(t, aggregation.ErrNegativeInput, testHandler.Flush())
		result.Observe(-1)
		require.Equal(t, aggregation.ErrNegativeInput, testHandler.Flush())
	})

	collected := sdk.Collect(ctx)

	require.Equal(t, 0, collected)
	require.EqualValues(t, map[string]float64{}, processor.Values())

	// check that the error condition was reset
	require.NoError(t, testHandler.Flush())
}

func TestObserverBatch(t *testing.T) {
	ctx := context.Background()
	meter, sdk, _, processor := newSDK(t)

	var floatGaugeObs metric.Float64GaugeObserver
	var intGaugeObs metric.Int64GaugeObserver
	var floatCounterObs metric.Float64CounterObserver
	var intCounterObs metric.Int64CounterObserver
	var floatUpDownCounterObs metric.Float64UpDownCounterObserver
	var intUpDownCounterObs metric.Int64UpDownCounterObserver

	var batch = Must(meter).NewBatchObserver(
		func(_ context.Context, result metric.BatchObserverResult) {
			result.Observe(
				[]attribute.KeyValue{
					attribute.String("A", "B"),
				},
				floatGaugeObs.Observation(1),
				floatGaugeObs.Observation(-1),
				intGaugeObs.Observation(-1),
				intGaugeObs.Observation(1),
				floatCounterObs.Observation(1000),
				intCounterObs.Observation(100),
				floatUpDownCounterObs.Observation(-1000),
				intUpDownCounterObs.Observation(-100),
			)
			result.Observe(
				[]attribute.KeyValue{
					attribute.String("C", "D"),
				},
				floatGaugeObs.Observation(-1),
				floatCounterObs.Observation(-1),
				floatUpDownCounterObs.Observation(-1),
			)
			result.Observe(
				nil,
				intGaugeObs.Observation(1),
				intGaugeObs.Observation(1),
				intCounterObs.Observation(10),
				floatCounterObs.Observation(1.1),
				intUpDownCounterObs.Observation(10),
			)
		})
	floatGaugeObs = batch.NewFloat64GaugeObserver("float.gauge.lastvalue")
	intGaugeObs = batch.NewInt64GaugeObserver("int.gauge.lastvalue")
	floatCounterObs = batch.NewFloat64CounterObserver("float.counterobserver.sum")
	intCounterObs = batch.NewInt64CounterObserver("int.counterobserver.sum")
	floatUpDownCounterObs = batch.NewFloat64UpDownCounterObserver("float.updowncounterobserver.sum")
	intUpDownCounterObs = batch.NewInt64UpDownCounterObserver("int.updowncounterobserver.sum")

	collected := sdk.Collect(ctx)

	require.Equal(t, collected, len(processor.Values()))

	require.EqualValues(t, map[string]float64{
		"float.counterobserver.sum//":    1.1,
		"float.counterobserver.sum/A=B/": 1000,
		"int.counterobserver.sum//":      10,
		"int.counterobserver.sum/A=B/":   100,

		"int.updowncounterobserver.sum/A=B/":   -100,
		"float.updowncounterobserver.sum/A=B/": -1000,
		"int.updowncounterobserver.sum//":      10,
		"float.updowncounterobserver.sum/C=D/": -1,

		"float.gauge.lastvalue/A=B/": -1,
		"float.gauge.lastvalue/C=D/": -1,
		"int.gauge.lastvalue//":      1,
		"int.gauge.lastvalue/A=B/":   1,
	}, processor.Values())
}

func TestRecordBatch(t *testing.T) {
	ctx := context.Background()
	meter, sdk, _, processor := newSDK(t)

	counter1 := Must(meter).NewInt64Counter("int64.sum")
	counter2 := Must(meter).NewFloat64Counter("float64.sum")
	histogram1 := Must(meter).NewInt64Histogram("int64.histogram")
	histogram2 := Must(meter).NewFloat64Histogram("float64.histogram")

	sdk.RecordBatch(
		ctx,
		[]attribute.KeyValue{
			attribute.String("A", "B"),
			attribute.String("C", "D"),
		},
		counter1.Measurement(1),
		counter2.Measurement(2),
		histogram1.Measurement(3),
		histogram2.Measurement(4),
	)

	sdk.Collect(ctx)

	require.EqualValues(t, map[string]float64{
		"int64.sum/A=B,C=D/":         1,
		"float64.sum/A=B,C=D/":       2,
		"int64.histogram/A=B,C=D/":   3,
		"float64.histogram/A=B,C=D/": 4,
	}, processor.Values())
}

// TestRecordPersistence ensures that a direct-called instrument that
// is repeatedly used each interval results in a persistent record, so
// that its encoded labels will be cached across collection intervals.
func TestRecordPersistence(t *testing.T) {
	ctx := context.Background()
	meter, sdk, selector, _ := newSDK(t)

	c := Must(meter).NewFloat64Counter("name.sum")
	uk := attribute.String("bound", "false")

	for i := 0; i < 100; i++ {
		c.Add(ctx, 1, uk)
		sdk.Collect(ctx)
	}

	require.Equal(t, 2, selector.newAggCount)
}

func TestIncorrectInstruments(t *testing.T) {
	// The Batch observe/record APIs are susceptible to
	// uninitialized instruments.
	var counter metric.Int64Counter
	var observer metric.Int64GaugeObserver

	ctx := context.Background()
	meter, sdk, _, processor := newSDK(t)

	// Now try with uninitialized instruments.
	meter.RecordBatch(ctx, nil, counter.Measurement(1))
	meter.NewBatchObserver(func(_ context.Context, result metric.BatchObserverResult) {
		result.Observe(nil, observer.Observation(1))
	})

	collected := sdk.Collect(ctx)
	require.Equal(t, metricsdk.ErrUninitializedInstrument, testHandler.Flush())
	require.Equal(t, 0, collected)

	// Now try with instruments from another SDK.
	var noopMeter metric.Meter
	counter = metric.Must(noopMeter).NewInt64Counter("name.sum")
	observer = metric.Must(noopMeter).NewBatchObserver(
		func(context.Context, metric.BatchObserverResult) {},
	).NewInt64GaugeObserver("observer")

	meter.RecordBatch(ctx, nil, counter.Measurement(1))
	meter.NewBatchObserver(func(_ context.Context, result metric.BatchObserverResult) {
		result.Observe(nil, observer.Observation(1))
	})

	collected = sdk.Collect(ctx)
	require.Equal(t, 0, collected)
	require.EqualValues(t, map[string]float64{}, processor.Values())
	require.Equal(t, metricsdk.ErrUninitializedInstrument, testHandler.Flush())
}

func TestSyncInAsync(t *testing.T) {
	ctx := context.Background()
	meter, sdk, _, processor := newSDK(t)

	counter := Must(meter).NewFloat64Counter("counter.sum")
	_ = Must(meter).NewInt64GaugeObserver("observer.lastvalue",
		func(ctx context.Context, result metric.Int64ObserverResult) {
			result.Observe(10)
			counter.Add(ctx, 100)
		},
	)

	sdk.Collect(ctx)

	require.EqualValues(t, map[string]float64{
		"counter.sum//":        100,
		"observer.lastvalue//": 10,
	}, processor.Values())
}
