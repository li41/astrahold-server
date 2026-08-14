package main

type phasedServerReport struct {
	measuredServerReport
	Phase       string              `json:"phase"`
	Convergence convergenceMetadata `json:"convergence"`
}

func withPhaseReport(report measuredServerReport, phase string, convergence convergenceMetadata) phasedServerReport {
	return phasedServerReport{measuredServerReport: report, Phase: phase, Convergence: convergence}
}
