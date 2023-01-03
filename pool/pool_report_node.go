package pool

import "sync-server/sstypes"

type PoolNodeChanReportCode = int32 // ANY SERVER REPORT CODE MUST BE NEGATIVE

const (
	REPORT_CODE_FINAL PoolNodeChanReportCode     = -1
	REPORT_CODE_NOT_CONNECTING PoolNodeChanReportCode = -2

	REPORT_CODE_DISCONNECT PoolNodeChanReportCode = PoolNodeChanReportCode(sstypes.SSMessage_DISCONNECT_REPORT)
)

func ReportCodeIsFromClient(reportCode PoolNodeChanReportCode) bool {
	return reportCode == REPORT_CODE_DISCONNECT
}

func ReportCodeRequiresReconnect(reportCode PoolNodeChanReportCode) bool {
	return reportCode == REPORT_CODE_NOT_CONNECTING || 
		reportCode == REPORT_CODE_DISCONNECT
}

// TODO report strategies / analysis / credibility score