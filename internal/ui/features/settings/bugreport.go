package settings

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// bundleState is the bug-report bundle's call in flight (Copy diagnostics
// reads "Collecting…" while it runs).
type bundleState struct{ pending bool }

// copyBundle collects the redacted bug-report bundle and puts it on the
// clipboard (Diagnostics.tsx's bundle mutation): the native motion report
// goes to Go first, so the bundle carries it; Go gathers the report,
// settings, crash output and recent log lines, scrubs secrets and saves a
// copy. The success toast has a Show action opening the log folder.
func (s *settings) copyBundle() {
	st := stateOf[bundleState](s, "diag:bundle")
	if st.pending {
		return
	}
	st.pending = true
	seam := s.diagSeams()
	// The policy answers on the main thread only.
	report, hasMotion := seam.nativeMotion(s)
	api, motion := seam.system(s), seam.motion(s)
	go func() {
		if hasMotion {
			motion.Report(report)
		}
		b, err := api.DiagnosticsBundle(context.Background())
		s.env.Post(func() {
			st.pending = false
			if err == nil && !s.env.Copy(b.Text) {
				err = errors.New("clipboard unavailable")
			}
			if err != nil {
				kit.ToastErrorf(i18n.T("Couldn't collect diagnostics"), i18n.ErrorMessage(err))
				return
			}
			s.bundleToast(b)
		})
	}()
}

// bundleToast says the bundle is on the clipboard and where its copy is.
func (s *settings) bundleToast(b services.DiagBundle) {
	spec := kit.ToastSpec{Title: i18n.T("Diagnostics copied"), Description: i18n.T("Tokens and your e-mail address are removed.")}
	if b.Path != "" {
		spec.Description = i18n.T("Tokens and your e-mail address are removed. A copy is saved as {file} in the log folder.", "file", filepath.Base(b.Path))
		spec.Action = i18n.T("Show")
		spec.OnAction = s.openLogFolder
	}
	kit.ShowToast(spec)
}

// openLogFolder opens the log folder; failure is a toast.
func (s *settings) openLogFolder() {
	api := s.diagSeams().system(s)
	go func() {
		err := api.OpenLogFolder()
		s.env.Post(func() {
			if err != nil {
				kit.ToastErrorf(i18n.T("Couldn't open the log folder"), i18n.ErrorMessage(err))
			}
		})
	}()
}
