package tailnet

import (
	"context"
	"errors"
	"net/url"

	"hello-tsnet/internal/routehelper"
	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
)

type Account struct {
	Name    string `json:"name,omitempty"`
	Login   string `json:"login,omitempty"`
	Tailnet string `json:"tailnet,omitempty"`
}

func (r *Runtime) applyNetworkStatus(network *ipnstate.Status) {
	if network == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	switch network.BackendState {
	case "Running":
		if r.status.State == StateSignedOut || network.Self == nil {
			return
		}
		profile := network.User[network.Self.UserID]
		account := &Account{Name: profile.DisplayName, Login: profile.LoginName}
		if network.CurrentTailnet != nil {
			account.Tailnet = network.CurrentTailnet.Name
		}
		local := displayDevice(network.Self, routehelper.AssignedAddresses(network.TailscaleIPs))
		local.Local = true
		if local.Name == "" {
			local.Name = "Hello TSNet"
		}
		r.status.State, r.status.AuthURL, r.status.Account = StateReady, "", account
		r.status.LocalDevice = &local
	case "NeedsLogin", "NeedsMachineAuth", "Stopped", "NoState":
		if r.status.State == StateReady {
			r.status = Status{State: StateSignedOut}
		}
	}
}

func (r *Runtime) Logout(ctx context.Context) error {
	r.accountMu.Lock()
	defer r.accountMu.Unlock()
	if r.Status().State != StateReady && !r.logoutRetry {
		return errors.New("当前没有已登录的账号")
	}
	if err := r.backend.Logout(ctx, r.logoutRetry); err != nil {
		r.logoutRetry = true
		return err
	}
	r.logoutRetry = false
	r.mu.Lock()
	r.status = Status{State: StateSignedOut}
	r.mu.Unlock()
	return nil
}

// Login opens a fresh interactive authorization flow after logout.
func (r *Runtime) Login(ctx context.Context) (string, error) {
	r.accountMu.Lock()
	defer r.accountMu.Unlock()

	r.mu.Lock()
	current := r.status
	switch {
	case !r.available:
		r.mu.Unlock()
		return "", errors.New("Tailscale 尚未就绪")
	case current.State == StateReady:
		r.mu.Unlock()
		return "", nil
	}
	r.status = Status{State: StatePending}
	r.mu.Unlock()
	r.logoutRetry = false

	link, err := r.backend.Login(ctx)
	if err == nil && link != "" {
		link = officialLoginURL(link)
		if link == "" {
			err = errors.New("无法确认授权页面地址")
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.status.State == StateReady {
		return "", nil
	}
	if err != nil {
		r.status = current
		return "", err
	}
	r.status.AuthURL = link
	return link, nil
}

func officialLoginURL(link string) string {
	parsed, err := url.Parse(link)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "login.tailscale.com" || parsed.User != nil {
		return ""
	}
	return parsed.String()
}

func (b *tsnetBackend) Logout(ctx context.Context, retry bool) error {
	client, err := b.server.LocalClient()
	if err != nil {
		return err
	}
	if retry {
		prefs, err := client.GetPrefs(ctx)
		if err != nil {
			return err
		}
		if prefs.LoggedOut && !prefs.WantRunning {
			// An unsuccessful logout can clear the control client's in-memory key.
			// Reload the stored profile while keeping it signed out before retrying.
			if err := client.Start(ctx, ipn.Options{}); err != nil {
				return err
			}
		}
	}
	return client.Logout(ctx)
}

func (b *tsnetBackend) Login(ctx context.Context) (string, error) {
	client, err := b.server.LocalClient()
	if err != nil {
		return "", err
	}
	watch, err := client.WatchIPNBus(ctx, ipn.NotifyNoNetMap)
	if err != nil {
		return "", err
	}
	defer watch.Close()
	status, err := client.StatusWithoutPeers(ctx)
	if err != nil || status.BackendState == "Running" {
		return "", err
	}
	// Logout removes the profile that held the configured node name.
	if _, err := client.EditPrefs(ctx, &ipn.MaskedPrefs{
		HostnameSet: true, Prefs: ipn.Prefs{Hostname: b.server.Hostname},
	}); err != nil {
		return "", err
	}
	if err := client.StartLoginInteractive(ctx); err != nil {
		return "", err
	}
	for {
		event, err := watch.Next()
		if err != nil {
			return "", err
		}
		if event.ErrMessage != nil {
			return "", errors.New("Tailscale 授权失败")
		}
		if event.BrowseToURL != nil {
			return *event.BrowseToURL, nil
		}
		if event.State != nil && *event.State == ipn.Running {
			return "", nil
		}
	}
}
