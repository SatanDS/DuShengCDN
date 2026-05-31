package controller

import (
	"dushengcdn/model"
	"dushengcdn/service"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

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
