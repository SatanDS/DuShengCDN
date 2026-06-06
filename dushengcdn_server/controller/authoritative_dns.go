package controller

import (
	"dushengcdn/model"
	"dushengcdn/service"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const maxDNSWorkerHeartbeatBodyBytes = 8 << 20

func GetDNSZones(c *gin.Context) {
	zones, err := service.ListAuthoritativeDNSZones()
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, zones)
}

func GetDNSZone(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	zone, err := service.GetAuthoritativeDNSZone(id)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, zone)
}

func CreateDNSZone(c *gin.Context) {
	var input service.DNSZoneInput
	if !decodeJSONRequest(c, &input) {
		return
	}
	zone, err := service.CreateAuthoritativeDNSZone(input)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, zone)
}

func UpdateDNSZone(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var input service.DNSZoneInput
	if !decodeJSONRequest(c, &input) {
		return
	}
	zone, err := service.UpdateAuthoritativeDNSZone(id, input)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, zone)
}

func DeleteDNSZone(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	if err := service.DeleteAuthoritativeDNSZone(id); err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, nil)
}

func GetDNSZoneRecords(c *gin.Context) {
	zoneID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	records, err := service.ListAuthoritativeDNSRecords(zoneID)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, records)
}

func CreateDNSZoneRecord(c *gin.Context) {
	zoneID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var input service.DNSRecordInput
	if !decodeJSONRequest(c, &input) {
		return
	}
	record, err := service.CreateAuthoritativeDNSRecord(zoneID, input)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, record)
}

func UpdateDNSRecord(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var input service.DNSRecordInput
	if !decodeJSONRequest(c, &input) {
		return
	}
	record, err := service.UpdateAuthoritativeDNSRecord(id, input)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, record)
}

func DeleteDNSRecord(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	if err := service.DeleteAuthoritativeDNSRecord(id); err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, nil)
}

func GetDNSWorkers(c *gin.Context) {
	workers, err := service.ListAuthoritativeDNSWorkers()
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, workers)
}

func GetDNSObservability(c *gin.Context) {
	hours, _ := strconv.Atoi(strings.TrimSpace(c.Query("hours")))
	zoneID, _ := strconv.ParseUint(strings.TrimSpace(c.Query("zone_id")), 10, 64)
	summary, err := service.GetAuthoritativeDNSObservabilitySummary(service.DNSObservabilitySummaryInput{
		Hours:    hours,
		ZoneID:   uint(zoneID),
		WorkerID: strings.TrimSpace(c.Query("worker_id")),
	})
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, summary)
}

func GetDNSGSLBSchedulingStates(c *gin.Context) {
	states, err := service.ListAuthoritativeDNSGSLBSchedulingStates()
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, states)
}

func GetDNSMigrationCandidates(c *gin.Context) {
	candidates, err := service.ListAuthoritativeDNSMigrationCandidates()
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, candidates)
}

func CheckDNSZoneDelegation(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	check, err := service.CheckAuthoritativeDNSZoneDelegation(id)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, check)
}

func CreateDNSWorker(c *gin.Context) {
	var input service.DNSWorkerInput
	if !decodeJSONRequest(c, &input) {
		return
	}
	worker, err := service.CreateAuthoritativeDNSWorker(input)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, worker)
}

func DeleteDNSWorker(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	if err := service.DeleteAuthoritativeDNSWorker(id); err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, nil)
}

func ProbeDNSWorker(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var input service.DNSWorkerProbeInput
	if err := decodeOptionalJSONBody(c.Request.Body, &input); err != nil {
		respondBadRequest(c, "invalid parameter")
		return
	}
	view, err := service.ProbeAuthoritativeDNSWorker(id, input)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, view)
}

func SimulateDNSGSLB(c *gin.Context) {
	var input service.DNSGSLBSimulationInput
	if !decodeJSONRequest(c, &input) {
		return
	}
	view, err := service.SimulateAuthoritativeDNSGSLB(input)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, view)
}

func GetDNSSnapshot(c *gin.Context) {
	worker, ok := authenticateDNSWorker(c)
	if !ok {
		return
	}
	snapshot, err := service.GetAuthoritativeDNSSnapshot(worker)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, snapshot)
}

func DNSWorkerHeartbeat(c *gin.Context) {
	worker, ok := authenticateDNSWorker(c)
	if !ok {
		return
	}
	if c.Request != nil && c.Request.Body != nil {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxDNSWorkerHeartbeatBodyBytes)
	}
	var input service.DNSWorkerHeartbeatInput
	if !decodeJSONRequest(c, &input) {
		return
	}
	view, err := service.RecordDNSWorkerHeartbeat(worker, input)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, view)
}

func GetDNSSourceDatabaseManifest(c *gin.Context) {
	if _, ok := authenticateDNSWorker(c); !ok {
		return
	}
	manifest, err := service.GetDNSSourceDatabaseMirrorManifest()
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "DNS source database mirror is not ready",
		})
		return
	}
	c.IndentedJSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    manifest,
	})
}

func DownloadDNSSourceDatabaseFile(c *gin.Context) {
	if _, ok := authenticateDNSWorker(c); !ok {
		return
	}
	file, meta, err := service.OpenDNSSourceDatabaseMirrorFile(c.Param("kind"), c.Param("name"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	defer file.Close()

	c.Header("X-DuShengCDN-Source-Database-SHA256", meta.SHA256)
	c.Header("X-DuShengCDN-Source-Database-Updated-At", meta.UpdatedAt.Format(time.RFC3339))
	c.Header("Content-Length", fmt.Sprintf("%d", meta.Size))
	c.FileAttachment(file.Name(), meta.Name)
}

func authenticateDNSWorker(c *gin.Context) (*model.DNSWorker, bool) {
	token := strings.TrimSpace(c.GetHeader("X-DNS-Worker-Token"))
	if token == "" {
		auth := strings.TrimSpace(c.GetHeader("Authorization"))
		if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			token = strings.TrimSpace(auth[7:])
		}
	}
	worker, err := service.AuthenticateDNSWorkerToken(token)
	if err != nil {
		respondUnauthorized(c, err.Error())
		return nil, false
	}
	return worker, true
}

func parseUintParam(c *gin.Context, key string) (uint, bool) {
	id, err := strconv.ParseUint(c.Param(key), 10, 64)
	if err != nil || id == 0 {
		respondBadRequest(c, "invalid parameter")
		return 0, false
	}
	return uint(id), true
}

func decodeJSONRequest(c *gin.Context, out any) bool {
	if err := decodeJSONBody(c.Request.Body, out); err != nil {
		respondBadRequest(c, "invalid parameter")
		return false
	}
	return true
}
