package main

import "net/http"

// 授权证件查看：仅站务/安保/管理员可查看证件原文，且每次查看写入审计链。
// 其余角色与所有常规页面只返回脱敏值。

func handleClaimCredential(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "station", "security") {
		errJSON(w, 403, "仅授权站务/安保可查看证件原文")
		return
	}
	id := pathID(r)
	var idc, didc string
	err := db.QueryRow(`SELECT COALESCE(claimant_id_card,''), COALESCE(delegate_id_card,'') FROM claims WHERE id=$1`, id).Scan(&idc, &didc)
	if err != nil {
		errJSON(w, 404, "认领单不存在")
		return
	}
	audit(u, "claim", id, "view_credential", "查看认领证件原文（核验用途）")
	writeJSON(w, 200, map[string]string{
		"claimant_id_card": idc,
		"delegate_id_card": didc,
	})
}

func handleReportCredential(w http.ResponseWriter, r *http.Request, u *User) {
	if !requireRole(u, "station", "security") {
		errJSON(w, 403, "仅授权站务/安保可查看证件原文")
		return
	}
	id := pathID(r)
	var idc string
	err := db.QueryRow(`SELECT COALESCE(passenger_id_card,'') FROM lost_reports WHERE id=$1`, id).Scan(&idc)
	if err != nil {
		errJSON(w, 404, "招领单不存在")
		return
	}
	audit(u, "report", id, "view_credential", "查看申报人证件原文（核验用途）")
	writeJSON(w, 200, map[string]string{"passenger_id_card": idc})
}
