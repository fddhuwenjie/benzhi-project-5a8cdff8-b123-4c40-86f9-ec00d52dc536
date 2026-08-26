package case_service

// 兼容历史导入路径；生产实现位于 internal/case_service。
import legacy "seismocal/internal/case_service"

type Service = legacy.Service

var New = legacy.New
var CanAccept = legacy.CanAccept
var RequireRevision = legacy.RequireRevision
