package service

import "marketingflow/internal/model"

// StepTemplate is the static definition of a workflow step. Creating a work item
// instantiates a model.WorkStep for each template of the chosen alur. The whole
// A..D flow is data-driven from the slices below — extending the system is just
// editing data, mirroring the Marketing department flowchart.
type StepTemplate struct {
	Code              string
	Alur              string
	Name              string
	Owner             string // PIC role responsible (see owner constants)
	CollabDept        string // supporting department for cross-team steps
	Phase             string // brief/produksi/review/approval/distribusi
	IsApproval        bool   // gated to Kepala Departemen Marketing
	RequiresBudget    bool   // pengajuan biaya iklan / top up saldo
	BudgetLabel       string
	NotifyDepartments bool
	SLADays           int
	MetadataKeys      []string // structured link fields the UI should collect
}

// Owner (PIC) labels — the internal Marketing team + the Kepala Departemen.
const (
	OwnerCopywriter = "Copywriter"
	OwnerDesign     = "Design Grafis"
	OwnerVideografer = "Videografer"
	OwnerTalentVid  = "Talent & Videografer"
	OwnerEditor     = "Video Editor"
	OwnerSocial     = "Social Media Specialist"
	OwnerSocialDM   = "Social Media / Digital Marketing"
	OwnerDigital    = "Digital Marketing"
	OwnerKadep      = "Kepala Departemen Marketing"
	OwnerSales      = "Departemen Sales"
)

// Supporting departments (kolaborasi).
const (
	DeptPerencanaan = "Departemen Perencanaan"
	DeptKeuangan    = "Departemen Keuangan"
	DeptSales       = "Departemen Sales"
)

// AlurLabels maps an alur code to its human label (also used by the UI).
var AlurLabels = map[string]string{
	"A": "A · Iklan Berbayar — Konten Hardsell / Desain Statis",
	"B": "B · Iklan Berbayar — Konten Video",
	"C": "C · Konten Organik — Carousel",
	"D": "D · Konten Organik — Video / Reels",
}

// ProcessA — Iklan Berbayar – Konten Hardsell / Desain Statis (10 langkah).
var ProcessA = []StepTemplate{
	{Code: "A1", Alur: "A", Phase: "brief", Owner: OwnerKadep, CollabDept: DeptPerencanaan, NotifyDepartments: true, SLADays: 2,
		Name: "Departemen Perencanaan menyerahkan gambar rumah ke Kepala Departemen Marketing, diteruskan ke Copywriter",
		MetadataKeys: []string{"link_gambar_rumah"}},
	{Code: "A2", Alur: "A", Phase: "brief", Owner: OwnerCopywriter, SLADays: 2,
		Name: "Copywriter membuat brief, tulisan, CTA, dan benchmark untuk Design Grafis",
		MetadataKeys: []string{"link_brief"}},
	{Code: "A3", Alur: "A", Phase: "produksi", Owner: OwnerDesign, SLADays: 3,
		Name: "Design Grafis mendesain konten hardsell menggunakan copy dan gambar rumah",
		MetadataKeys: []string{"link_desain"}},
	{Code: "A4", Alur: "A", Phase: "review", Owner: OwnerCopywriter, SLADays: 1,
		Name: "Hasil desain dikirim ke Copywriter untuk review & revisi"},
	{Code: "A5", Alur: "A", Phase: "review", Owner: OwnerDesign, SLADays: 2,
		Name: "Jika ada revisi, Design Grafis melakukan perbaikan desain"},
	{Code: "A6", Alur: "A", Phase: "approval", Owner: OwnerCopywriter, IsApproval: true, SLADays: 1,
		Name: "Copywriter mengirim hasil final ke Kepala Departemen Marketing untuk approval"},
	{Code: "A7", Alur: "A", Phase: "approval", Owner: OwnerKadep, CollabDept: DeptKeuangan, NotifyDepartments: true,
		RequiresBudget: true, BudgetLabel: "Biaya Iklan", SLADays: 2,
		Name: "Kepala Departemen Marketing mengajukan biaya iklan ke Departemen Keuangan",
		MetadataKeys: []string{"tanggal_pengajuan"}},
	{Code: "A8", Alur: "A", Phase: "approval", Owner: OwnerKadep, RequiresBudget: true, BudgetLabel: "Top Up Saldo", SLADays: 1,
		Name: "Setelah dana tersedia, Kepala Departemen Marketing melakukan top up saldo iklan",
		MetadataKeys: []string{"platform", "tanggal_topup"}},
	{Code: "A9", Alur: "A", Phase: "distribusi", Owner: OwnerDigital, SLADays: 1,
		Name: "Hasil desain yang di-approve dikirim ke Digital Marketing untuk tayang di Meta Ads",
		MetadataKeys: []string{"link_meta_ads", "tanggal_tayang"}},
	{Code: "A10", Alur: "A", Phase: "distribusi", Owner: OwnerSales, CollabDept: DeptSales, NotifyDepartments: true, SLADays: 1,
		Name: "Lead masuk ke Mekari Qontak dan langsung di-follow up oleh Departemen Sales"},
}

// ProcessB — Iklan Berbayar – Konten Video (12 langkah).
var ProcessB = []StepTemplate{
	{Code: "B1", Alur: "B", Phase: "brief", Owner: OwnerCopywriter, SLADays: 2,
		Name: "Copywriter membuat brief shooting iklan untuk Talent dan Videografer",
		MetadataKeys: []string{"link_brief"}},
	{Code: "B2", Alur: "B", Phase: "produksi", Owner: OwnerTalentVid, SLADays: 3,
		Name: "Talent dan Videografer melakukan shooting konten iklan",
		MetadataKeys: []string{"link_footage_icloud", "tanggal_shooting"}},
	{Code: "B3", Alur: "B", Phase: "produksi", Owner: OwnerCopywriter, SLADays: 1,
		Name: "Copywriter mengirim brief ke Video Editor"},
	{Code: "B4", Alur: "B", Phase: "produksi", Owner: OwnerEditor, SLADays: 3,
		Name: "Video Editor mengambil hasil shooting di iCloud dan mulai editing",
		MetadataKeys: []string{"link_draft_video"}},
	{Code: "B5", Alur: "B", Phase: "produksi", Owner: OwnerEditor, SLADays: 3,
		Name: "Khusus proyek baru: gunakan video render animasi perumahan jika rumah contoh belum tersedia",
		MetadataKeys: []string{"link_render_animasi"}},
	{Code: "B6", Alur: "B", Phase: "review", Owner: OwnerCopywriter, SLADays: 1,
		Name: "Hasil video dikirim ke Copywriter untuk review & revisi"},
	{Code: "B7", Alur: "B", Phase: "review", Owner: OwnerEditor, SLADays: 2,
		Name: "Jika ada revisi, Video Editor melakukan perbaikan"},
	{Code: "B8", Alur: "B", Phase: "approval", Owner: OwnerCopywriter, IsApproval: true, SLADays: 1,
		Name: "Copywriter mengirim hasil final ke Kepala Departemen Marketing untuk approval"},
	{Code: "B9", Alur: "B", Phase: "approval", Owner: OwnerKadep, CollabDept: DeptKeuangan, NotifyDepartments: true,
		RequiresBudget: true, BudgetLabel: "Biaya Iklan", SLADays: 2,
		Name: "Kepala Departemen Marketing mengajukan biaya iklan ke Departemen Keuangan",
		MetadataKeys: []string{"tanggal_pengajuan"}},
	{Code: "B10", Alur: "B", Phase: "approval", Owner: OwnerKadep, RequiresBudget: true, BudgetLabel: "Top Up Saldo", SLADays: 1,
		Name: "Setelah dana tersedia, Kepala Departemen Marketing melakukan top up saldo iklan",
		MetadataKeys: []string{"platform", "tanggal_topup"}},
	{Code: "B11", Alur: "B", Phase: "distribusi", Owner: OwnerDigital, SLADays: 1,
		Name: "Video yang di-approve dikirim ke Digital Marketing untuk tayang di Meta Ads",
		MetadataKeys: []string{"link_meta_ads", "tanggal_tayang"}},
	{Code: "B12", Alur: "B", Phase: "distribusi", Owner: OwnerSales, CollabDept: DeptSales, NotifyDepartments: true, SLADays: 1,
		Name: "Lead masuk ke Mekari Qontak dan langsung di-follow up oleh Departemen Sales"},
}

// ProcessC — Konten Organik – Carousel (9 langkah).
var ProcessC = []StepTemplate{
	{Code: "C1", Alur: "C", Phase: "brief", Owner: OwnerCopywriter, SLADays: 1,
		Name: "Copywriter memberikan brief kepada Design Grafis",
		MetadataKeys: []string{"link_brief"}},
	{Code: "C2", Alur: "C", Phase: "produksi", Owner: OwnerDesign, SLADays: 2,
		Name: "Design Grafis membuat desain carousel",
		MetadataKeys: []string{"link_desain"}},
	{Code: "C3", Alur: "C", Phase: "review", Owner: OwnerCopywriter, SLADays: 1,
		Name: "Copywriter melakukan review & revisi"},
	{Code: "C4", Alur: "C", Phase: "review", Owner: OwnerDesign, SLADays: 1,
		Name: "Jika ada revisi, Design Grafis melakukan perbaikan"},
	{Code: "C5", Alur: "C", Phase: "approval", Owner: OwnerCopywriter, IsApproval: true, SLADays: 1,
		Name: "Copywriter mengirim hasil final ke Kepala Departemen Marketing untuk approval"},
	{Code: "C6", Alur: "C", Phase: "distribusi", Owner: OwnerSocial, SLADays: 1,
		Name: "Setelah di-approve, Social Media Specialist membuat caption dan menjadwalkan posting",
		MetadataKeys: []string{"caption", "jadwal_posting"}},
	{Code: "C7", Alur: "C", Phase: "distribusi", Owner: OwnerSocial, SLADays: 1,
		Name: "Konten diposting",
		MetadataKeys: []string{"link_postingan"}},
	{Code: "C8", Alur: "C", Phase: "distribusi", Owner: OwnerSocialDM, SLADays: 1,
		Name: "Social Media Specialist dan/atau Digital Marketing membantu membalas komentar"},
	{Code: "C9", Alur: "C", Phase: "distribusi", Owner: OwnerSocial, SLADays: 1,
		Name: "Social Media Specialist melakukan posting story setiap hari"},
}

// ProcessD — Konten Organik – Video / Reels (11 langkah).
var ProcessD = []StepTemplate{
	{Code: "D1", Alur: "D", Phase: "brief", Owner: OwnerCopywriter, SLADays: 1,
		Name: "Copywriter membuat brief untuk Talent dan Videografer",
		MetadataKeys: []string{"link_brief"}},
	{Code: "D2", Alur: "D", Phase: "produksi", Owner: OwnerTalentVid, SLADays: 2,
		Name: "Talent dan Videografer melakukan shooting",
		MetadataKeys: []string{"link_footage_icloud", "tanggal_shooting"}},
	{Code: "D3", Alur: "D", Phase: "produksi", Owner: OwnerCopywriter, SLADays: 1,
		Name: "Copywriter mengirim brief ke Video Editor"},
	{Code: "D4", Alur: "D", Phase: "produksi", Owner: OwnerEditor, SLADays: 2,
		Name: "Video Editor mengambil hasil shooting di iCloud dan mengedit sesuai brief",
		MetadataKeys: []string{"link_draft_video"}},
	{Code: "D5", Alur: "D", Phase: "review", Owner: OwnerCopywriter, SLADays: 1,
		Name: "Copywriter melakukan review & revisi"},
	{Code: "D6", Alur: "D", Phase: "review", Owner: OwnerEditor, SLADays: 1,
		Name: "Jika ada revisi, Video Editor melakukan perbaikan"},
	{Code: "D7", Alur: "D", Phase: "approval", Owner: OwnerCopywriter, IsApproval: true, SLADays: 1,
		Name: "Copywriter mengirim hasil final ke Kepala Departemen Marketing untuk approval"},
	{Code: "D8", Alur: "D", Phase: "distribusi", Owner: OwnerSocial, SLADays: 1,
		Name: "Setelah di-approve, Social Media Specialist membuat caption dan menjadwalkan posting",
		MetadataKeys: []string{"caption", "jadwal_posting"}},
	{Code: "D9", Alur: "D", Phase: "distribusi", Owner: OwnerSocial, SLADays: 1,
		Name: "Konten diposting",
		MetadataKeys: []string{"link_postingan"}},
	{Code: "D10", Alur: "D", Phase: "distribusi", Owner: OwnerSocialDM, SLADays: 1,
		Name: "Social Media Specialist dan/atau Digital Marketing membantu membalas komentar"},
	{Code: "D11", Alur: "D", Phase: "distribusi", Owner: OwnerSocial, SLADays: 1,
		Name: "Social Media Specialist melakukan posting story setiap hari"},
}

// CatalogFor returns the step templates for a single alur, in order. A work item
// is seeded from exactly one alur.
func CatalogFor(alur model.Alur) []StepTemplate {
	switch alur {
	case model.AlurHardsell:
		return ProcessA
	case model.AlurVideoAd:
		return ProcessB
	case model.AlurCarousel:
		return ProcessC
	case model.AlurReels:
		return ProcessD
	default:
		return nil
	}
}

// toModel converts a template into a persistable WorkStep.
func (t StepTemplate) toModel(workItemID uint, sequence int) model.WorkStep {
	return model.WorkStep{
		WorkItemID:        workItemID,
		Code:              t.Code,
		Alur:              t.Alur,
		Name:              t.Name,
		Sequence:          sequence,
		Phase:             t.Phase,
		Owner:             t.Owner,
		CollabDept:        t.CollabDept,
		Status:            model.StatusPending,
		IsApproval:        t.IsApproval,
		RequiresBudget:    t.RequiresBudget,
		BudgetLabel:       t.BudgetLabel,
		NotifyDepartments: t.NotifyDepartments,
		SLADays:           t.SLADays,
	}
}
