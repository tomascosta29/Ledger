package screens

import "github.com/tomascosta29/Ledger/internal/application/ports"

func overlayFindAll() ports.OverlayFindOptions {
	return ports.OverlayFindOptions{
		Sort:  ports.OverlaySortByDate,
		Order: ports.SortDesc,
		Limit: 200,
	}
}
