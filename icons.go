package main

import "github.com/egoist/mygo/ui"

// svgIcon builds a 24×24 stroke icon from its shapes, in the color of the text
// that contains it.
func svgIcon(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

// windowIcon keeps caption glyphs balanced on a smaller, finer stroke grid.
func windowIcon(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.25" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

var (
	icoDownload   = svgIcon(`<path d="M12 3v12"/><path d="m7 10 5 5 5-5"/><path d="M5 21h14"/>`)
	icoPlay       = svgIcon(`<path d="M7 4.5v15l13-7.5z"/>`)
	icoPause      = svgIcon(`<path d="M8.5 5v14"/><path d="M15.5 5v14"/>`)
	icoX          = svgIcon(`<path d="m6 6 12 12"/><path d="m18 6-12 12"/>`)
	icoMinimize   = windowIcon(`<path d="M3.5 8.5h9"/>`)
	icoMaximize   = windowIcon(`<rect x="3.5" y="3.5" width="9" height="9" rx="1"/>`)
	icoRestore    = windowIcon(`<path d="M5.5 2.5h7a1 1 0 0 1 1 1v7"/><rect x="2.5" y="5.5" width="8" height="8" rx="1"/>`)
	icoWindowX    = windowIcon(`<path d="m4 4 8 8m0-8-8 8"/>`)
	icoRefresh    = svgIcon(`<path d="M21 12a9 9 0 1 1-2.6-6.4"/><path d="M21 3v6h-6"/>`)
	icoFolder     = svgIcon(`<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>`)
	icoFolderPlus = svgIcon(`<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/><path d="M12 11v6"/><path d="M9 14h6"/>`)
	icoFolderOpen = svgIcon(`<path d="m6 14 1.5-4.5A2 2 0 0 1 9.4 8H21l-2 7a2 2 0 0 1-1.9 1.5H4a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h4l2 2h5a2 2 0 0 1 2 2v1"/>`)
	icoSettings   = svgIcon(`<path d="M4 6h16"/><path d="M4 12h16"/><path d="M4 18h16"/><circle cx="9" cy="6" r="2.2"/><circle cx="15" cy="12" r="2.2"/><circle cx="8" cy="18" r="2.2"/>`)
	icoSun        = svgIcon(`<circle cx="12" cy="12" r="4"/><path d="M12 2v2"/><path d="M12 20v2"/><path d="m4.9 4.9 1.4 1.4"/><path d="m17.7 17.7 1.4 1.4"/><path d="M2 12h2"/><path d="M20 12h2"/><path d="m6.3 17.7-1.4 1.4"/><path d="m19.1 4.9-1.4 1.4"/>`)
	icoMoon       = svgIcon(`<path d="M21 12.8A9 9 0 1 1 11.2 3 7 7 0 0 0 21 12.8z"/>`)
	icoMonitor    = svgIcon(`<rect x="3" y="4" width="18" height="12" rx="2"/><path d="M8 20h8"/><path d="M12 16v4"/>`)
	icoSearch     = svgIcon(`<circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/>`)
	icoLink       = svgIcon(`<path d="M10 13a5 5 0 0 0 7.5.5l3-3a5 5 0 0 0-7-7l-1.7 1.7"/><path d="M14 11a5 5 0 0 0-7.5-.5l-3 3a5 5 0 0 0 7 7l1.7-1.7"/>`)
	icoClipboard  = svgIcon(`<rect x="8" y="2" width="8" height="4" rx="1"/><path d="M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2"/>`)
	icoPlus       = svgIcon(`<path d="M12 5v14"/><path d="M5 12h14"/>`)
	icoTrash      = svgIcon(`<path d="M3 6h18"/><path d="M8 6V4h8v2"/><path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/><path d="M10 11v6"/><path d="M14 11v6"/>`)
	icoCheck      = svgIcon(`<path d="m5 13 4 4L19 7"/>`)
	icoAlert      = svgIcon(`<path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z"/><path d="M12 9v4"/><path d="M12 17h.01"/>`)
	icoBell       = svgIcon(`<path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9"/><path d="M10.3 21a1.94 1.94 0 0 0 3.4 0"/>`)
	icoInbox      = svgIcon(`<path d="M22 12h-6l-2 3h-4l-2-3H2"/><path d="M5.5 5.1 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.5-6.9A2 2 0 0 0 16.7 4H7.3a2 2 0 0 0-1.8 1.1z"/>`)
	icoClock      = svgIcon(`<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>`)
	icoList       = svgIcon(`<path d="M8 6h13"/><path d="M8 12h13"/><path d="M8 18h13"/><path d="M3 6h.01"/><path d="M3 12h.01"/><path d="M3 18h.01"/>`)
	icoGrid       = svgIcon(`<rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/>`)
	icoGlobe      = svgIcon(`<circle cx="12" cy="12" r="9"/><path d="M3 12h18"/><path d="M12 3a15 15 0 0 1 0 18a15 15 0 0 1 0-18z"/>`)
	icoShield     = svgIcon(`<path d="M12 3 5 6v5c0 4.5 3 8 7 10 4-2 7-5.5 7-10V6z"/><path d="m9 12 2 2 4-4"/>`)
	icoCopy       = svgIcon(`<rect x="9" y="9" width="11" height="11" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>`)
	icoChevron    = svgIcon(`<path d="m6 9 6 6 6-6"/>`)
	icoMore       = svgIcon(`<circle cx="5" cy="12" r="1"/><circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/>`)
	icoFile       = svgIcon(`<path d="M14 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z"/><path d="M14 3v6h6"/>`)
	icoExternal   = svgIcon(`<path d="M14 3h7v7"/><path d="M10 14 21 3"/><path d="M21 14v5a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5"/>`)
	icoZap        = svgIcon(`<path d="M13 2 4 14h7l-1 8 9-12h-7z"/>`)
	icoInfo       = svgIcon(`<circle cx="12" cy="12" r="9"/><path d="M12 11v5"/><path d="M12 8h.01"/>`)
	icoSliders    = svgIcon(`<path d="M4 8h10"/><path d="M18 8h2"/><path d="M4 16h4"/><path d="M12 16h8"/><circle cx="16" cy="8" r="2"/><circle cx="10" cy="16" r="2"/>`)
	icoServer     = svgIcon(`<rect x="3" y="4" width="18" height="7" rx="2"/><rect x="3" y="13" width="18" height="7" rx="2"/><path d="M7 7.5h.01"/><path d="M7 16.5h.01"/>`)
)
