package httpapi

import "benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/mission"

func fmtQueryError() error { return mission.Invalid("query", "分页参数必须为非负整数") }
