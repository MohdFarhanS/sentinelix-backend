package domain

import (
	"context"
	"errors"
)

// Overall status levels untuk status page publik, ala Statuspage.io.
// 3 level (bukan cuma operational/outage) supaya user tahu bedanya
// "semua down total" vs "cuma sebagian yang bermasalah" — informasi ini
// penting buat status page publik yang dibaca orang luar tim.
const (
	OverallStatusOperational         = "operational"
	OverallStatusDegradedPerformance = "degraded_performance"
	OverallStatusMajorOutage         = "major_outage"
)

// ErrProjectSlugNotFound dipakai khusus lookup by slug (public, tanpa
// auth) — SENGAJA terpisah dari ErrProjectNotFound (dashboard, lookup by
// ID + ownership check). Pesan error yang tepat beda konteks: yang ini
// murni "slug tidak match project manapun", tidak ada notion of
// ownership/permission sama sekali karena endpoint ini publik.
var ErrProjectSlugNotFound = errors.New("project not found for given slug")

// StatusMonitorInfo adalah representasi 1 monitor di status page publik.
// SENGAJA bukan alias/reuse Monitor langsung — field yang di-expose ke
// publik jauh lebih sempit (tidak ada ChannelTarget, ChannelType, dst,
// yang bisa jadi info sensitif kalau bocor ke publik).
//
// IsUp adalah bool sederhana (bukan 3-state) sesuai kontrak response di
// 04-API-DESIGN.md §9. Monitor berstatus "unknown" (belum sempat
// di-checker) di-map ke IsUp=true (ditampilkan netral, TIDAK dianggap
// down) — lihat MonitorIsUp().
type StatusMonitorInfo struct {
	Name      string
	IsUp      bool
	Uptime30d float64 // persentase, 0-100. Dihitung on-the-fly dari monitor_checks 30 hari terakhir (lihat status_repo.go).
}

// StatusPageResponse adalah read model utuh buat GET /status/:slug
// (04-API-DESIGN.md §9). Bukan entity domain baru dengan repository CRUD
// sendiri — ini murni bentuk data yang dikembalikan satu operasi
// gabungan di status_repo.go (project by slug + monitors + uptime_30d).
type StatusPageResponse struct {
	ProjectName   string
	OverallStatus string
	Monitors      []StatusMonitorInfo
}

// MonitorIsUp menerjemahkan status mentah monitors.status ke bool publik.
// "unknown" SENGAJA di-map ke true (ditampilkan seolah normal) — bukan
// karena beneran up, tapi karena belum ada bukti sebaliknya, dan
// menampilkannya sebagai "down" ke publik akan jadi false alarm buat
// monitor yang baru dibuat dan belum sempat di-ping (lihat
// ComputeOverallStatus untuk exclude yang lebih ketat di level agregat).
func MonitorIsUp(status string) bool {
	return status != MonitorStatusDown
}

// ComputeOverallStatus menghitung OverallStatus dari status MENTAH tiap
// monitor (bukan dari StatusMonitorInfo.IsUp yang sudah di-collapse jadi
// bool) — supaya monitor "unknown" bisa di-EXCLUDE total dari
// perhitungan, bukan ikut dihitung sebagai up ataupun down.
//
// Logic ala Statuspage.io, dihitung HANYA dari monitor yang statusnya
// definitif (up/down):
//   - Tidak ada monitor definitif sama sekali (kosong, atau semua masih
//     unknown) -> operational (tidak ada bukti masalah)
//   - Semua monitor definitif berstatus down -> major_outage
//   - Campuran (ada yang up, ada yang down) -> degraded_performance
//
// Diletakkan sebagai function di domain (bukan di usecase/repo) supaya
// logic ini gampang di-unit-test terisolasi tanpa mock DB apapun — pure
// function murni dari data yang sudah ada.
func ComputeOverallStatus(rawStatuses []string) string {
	upCount, downCount := 0, 0
	for _, s := range rawStatuses {
		switch s {
		case MonitorStatusUp:
			upCount++
		case MonitorStatusDown:
			downCount++
		}
		// MonitorStatusUnknown sengaja diabaikan — tidak masuk hitungan.
	}

	total := upCount + downCount
	switch {
	case total == 0:
		return OverallStatusOperational
	case downCount == total:
		return OverallStatusMajorOutage
	case downCount == 0:
		return OverallStatusOperational
	default:
		return OverallStatusDegradedPerformance
	}
}

// StatusRepository didefinisikan di domain, diimplementasikan di
// internal/repository/postgres (status_repo.go). Method tunggal ini
// sengaja menggabungkan project+monitors+uptime jadi satu operasi
// (2 query internal, bukan 3 repository terpisah) — status page publik
// butuh semua data ini sekaligus, dan endpoint ini di-ISR cache di
// frontend, jadi tidak masalah kalau operasinya agak "gemuk".
type StatusRepository interface {
	GetProjectStatusBySlug(ctx context.Context, slug string) (*ProjectStatusData, error)
}

// ProjectStatusData adalah hasil RAW dari repository — Name per monitor
// di sini SUDAH di-fallback ke URL (dilakukan repo, bukan Go struct
// Monitor karena repo ini tidak query full Monitor, cuma kolom yang
// perlu). RawStatus MASIH string mentah (up/down/unknown), BELUM
// di-map ke bool publik — itu tanggung jawab usecase
// (get_status_page.go), supaya ComputeOverallStatus bisa exclude
// "unknown" dengan benar sebelum di-collapse jadi bool.
type ProjectStatusData struct {
	ProjectName string
	Monitors    []MonitorStatusData
}

type MonitorStatusData struct {
	Name      string
	RawStatus string
	Uptime30d float64
}