package metrics

import (
	"fmt"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	counterLabelCompName = "component_name"
	counterLabelAnnName  = "annotation_name"

	NMOInUse    = float64(1)
	NMONotInUse = float64(0)
)

type metricDesc struct {
	fqName          string
	help            string
	constLabelPairs []string
	initFunc        func(metricDesc) prometheus.Collector
}

func (md metricDesc) init() prometheus.Collector {
	return md.initFunc(md)
}

// HcoMetrics wrapper for all hco metrics
var HcoMetrics = func() hcoMetrics {
	metricDescList := map[string]metricDesc{
		"overwrittenModifications": {
			fqName:          "kubevirt_hco_out_of_band_modifications_count",
			help:            "Count of out-of-band modifications overwritten by HCO",
			constLabelPairs: []string{counterLabelCompName},
			initFunc: func(md metricDesc) prometheus.Collector {
				return prometheus.NewCounterVec(
					prometheus.CounterOpts{
						Name: md.fqName,
						Help: md.help,
					},
					md.constLabelPairs,
				)
			},
		},
		"unsafeModifications": {
			fqName:          "kubevirt_hco_unsafe_modification_count",
			help:            "Count of unsafe modifications in the HyperConverged annotations",
			constLabelPairs: []string{counterLabelAnnName},
			initFunc: func(md metricDesc) prometheus.Collector {
				return prometheus.NewGaugeVec(
					prometheus.GaugeOpts{
						Name: md.fqName,
						Help: md.help,
					},
					md.constLabelPairs,
				)
			},
		},
		"nmoInUse": {
			fqName: "kubevirt_hco_nmo_in_use",
			help:   "Indicates whether integrated Node Maintenance Operator is being used (1) or not (0)",
			initFunc: func(md metricDesc) prometheus.Collector {
				return prometheus.NewGauge(
					prometheus.GaugeOpts{
						Name: md.fqName,
						Help: md.help,
					},
				)
			},
		},
	}

	metricList := make(map[string]prometheus.Collector)
	for k, md := range metricDescList {
		metricList[k] = md.init()
	}

	return hcoMetrics{
		metricDescList: metricDescList,
		metricList:     metricList,
	}
}()

// hcoMetrics holds all HCO metrics
type hcoMetrics struct {
	// overwrittenModifications counts out-of-band modifications overwritten by HCO
	metricDescList map[string]metricDesc
	metricList     map[string]prometheus.Collector
}

func init() {
	HcoMetrics.init()
}

func (hm hcoMetrics) init() {
	collectors := make([]prometheus.Collector, len(hm.metricList))
	i := 0
	for _, v := range hm.metricList {
		collectors[i] = v
		i++
	}
	metrics.Registry.MustRegister(collectors...)
}

func (hm *hcoMetrics) GetMetricValue(metricName string, label prometheus.Labels) (float64, error) {
	var res = &dto.Metric{}
	metric, found := hm.metricList[metricName]
	if !found {
		return 0, fmt.Errorf("unknown metric name %s", metricName)
	}
	switch m := metric.(type) {
	case *prometheus.CounterVec:
		err := m.With(label).Write(res)
		if err != nil {
			return 0, err
		}
		return res.Counter.GetValue(), nil
	case *prometheus.GaugeVec:
		err := m.With(label).Write(res)
		if err != nil {
			return 0, err
		}
		return res.Gauge.GetValue(), nil
	case prometheus.Gauge:
		err := m.Write(res)
		if err != nil {
			return 0, err
		}
		return res.Gauge.GetValue(), nil
	default:
		return 0, fmt.Errorf("%s is with unknown metric type", metricName)
	}
}

func (hm *hcoMetrics) IncMetric(metricName string, label prometheus.Labels) error {
	metric, found := hm.metricList[metricName]
	if !found {
		return fmt.Errorf("unknown metric name %s", metricName)
	}
	switch m := metric.(type) {
	case *prometheus.CounterVec:
		m.With(label).Inc()
		return nil
	case *prometheus.GaugeVec:
		m.With(label).Inc()
		return nil
	default:
		return fmt.Errorf("%s is with unknown metric type", metricName)
	}
}

func (hm *hcoMetrics) SetMetric(metricName string, label prometheus.Labels, value float64) error {
	metric, found := hm.metricList[metricName]
	if !found {
		return unknownMetricNameError(metricName)
	}

	switch m := metric.(type) {
	case *prometheus.GaugeVec:
		m.With(label).Set(value)

	case prometheus.Gauge:
		m.Set(value)

	default:
		return unknownMetricTypeError(metricName)
	}

	return nil
}

// IncOverwrittenModifications increments counter by 1
func (hm *hcoMetrics) IncOverwrittenModifications(kind, name string) error {
	return hm.IncMetric("overwrittenModifications", getLabelsForObj(kind, name))
}

// GetOverwrittenModificationsCount returns current value of counter. If error is not nil then value is undefined
func (hm *hcoMetrics) GetOverwrittenModificationsCount(kind, name string) (float64, error) {
	return hm.GetMetricValue("overwrittenModifications", getLabelsForObj(kind, name))
}

// SetUnsafeModificationCount sets the counter to the required number
func (hm *hcoMetrics) SetUnsafeModificationCount(count int, unsafeAnnotation string) error {
	return hm.SetMetric("unsafeModifications", getLabelsForUnsafeAnnotation(unsafeAnnotation), float64(count))
}

// GetUnsafeModificationsCount returns current value of counter. If error is not nil then value is undefined
func (hm *hcoMetrics) GetUnsafeModificationsCount(unsafeAnnotation string) (float64, error) {
	return hm.GetMetricValue("unsafeModifications", getLabelsForUnsafeAnnotation(unsafeAnnotation))
}

// SetNmoInUseGauge sets the metric to 1 to indicate NMO is in use
func (hm *hcoMetrics) SetNmoInUseGauge() error {
	return hm.SetMetric("nmoInUse", nil, NMOInUse)
}

// SetNmoNotInUseGauge sets the metric to 0 to indicate NMO is not in use
func (hm *hcoMetrics) SetNmoNotInUseGauge() error {
	return hm.SetMetric("nmoInUse", nil, NMONotInUse)
}

// IsNmoInUse returns current value of the metric. If error is not nil then value is undefined
func (hm *hcoMetrics) IsNmoInUse() (bool, error) {
	val, err := hm.GetMetricValue("nmoInUse", nil)
	if err != nil {
		return false, err
	}
	return val == NMOInUse, nil
}

func getLabelsForObj(kind string, name string) prometheus.Labels {
	return prometheus.Labels{counterLabelCompName: strings.ToLower(kind + "/" + name)}
}

func getLabelsForUnsafeAnnotation(unsafeAnnotation string) prometheus.Labels {
	return prometheus.Labels{counterLabelAnnName: strings.ToLower(unsafeAnnotation)}
}

type MetricDescription struct {
	FqName string
	Help   string
}

func (hm hcoMetrics) GetMetricDesc() []MetricDescription {
	res := make([]MetricDescription, len(hm.metricDescList))
	i := 0
	for _, md := range hm.metricDescList {
		res[i] = MetricDescription{FqName: md.fqName, Help: md.help}
		i++
	}

	return res
}

func unknownMetricNameError(metricName string) error {
	return fmt.Errorf("unknown metric name %s", metricName)
}

func unknownMetricTypeError(metricName string) error {
	return fmt.Errorf("%s is with unknown metric type", metricName)
}
