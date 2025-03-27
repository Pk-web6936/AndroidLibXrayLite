package libv2ray

// ... (other imports)
import "github.com/pkg/errors"

// ... (rest of the code)

// handleResolve handles the resolution process for domains.
func (v *V2RayPoint) handleResolve() {
	select {
	case <-v.dialer.ResolveChan():
		if !v.dialer.IsVServerReady() {
			log.Println("vServer cannot resolve, shutting down")
			v.StopLoop()
			v.SupportSet.Shutdown()
		}
	case <-v.closeChan:
	}
}

// StopLoop stops the V2Ray main loop.
func (v *V2RayPoint) StopLoop() error {
	v.v2rayOP.Lock()
	defer v.v2rayOP.Unlock()

	if v.IsRunning {
		close(v.closeChan)
		v.shutdownInit()
		v.SupportSet.OnEmitStatus(0, "Closed")
	}
	return nil
}

// shutdownInit shuts down the V2Ray instance and cleans up resources.
func (v *V2RayPoint) shutdownInit() {
	if v.Vpoint != nil {
		v.Vpoint.Close()
		v.Vpoint = nil
	}
	v.IsRunning = false
	v.statsManager = nil
}

// pointloop sets up and starts the V2Ray core.
func (v *V2RayPoint) pointloop() error {
	log.Println("Loading core config")
	config, err := v2serial.LoadJSONConfig(strings.NewReader(v.ConfigureFileContent))
	if err != nil {
		return errors.Wrap(err, "failed to load core config")
	}

	log.Println("Creating new core instance")
	v.Vpoint, err = v2core.New(config)
	if err != nil {
		return errors.Wrap(err, "failed to create core instance")
	}
	v.statsManager = v.Vpoint.GetFeature(v2stats.ManagerType()).(v2stats.Manager)

	log.Println("Starting core")
	v.IsRunning = true
	if err := v.Vpoint.Start(); err != nil {
		v.IsRunning = false
		return errors.Wrap(err, "failed to start core")
	}

	v.SupportSet.Prepare()
	v.SupportSet.Setup("")
	v.SupportSet.OnEmitStatus(0, "Running")
	return nil
}

// MeasureDelay measures the delay to a given URL.
func (v *V2RayPoint) MeasureDelay(url string) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	go func() {
		select {
		case <-v.closeChan:
			cancel()
		case <-ctx.Done():
		}
	}()

	return measureInstDelay(ctx, v.Vpoint, url)
}

// measureInstDelay measures the delay for an instance to a given URL.
func measureInstDelay(ctx context.Context, inst *v2core.Instance, url string) (int64, error) {
	if inst == nil {
		return -1, errors.New("core instance is nil")
	}

	tr := &http.Transport{
		TLSHandshakeTimeout: 6 * time.Second,
		DisableKeepAlives:   true,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dest, err := v2net.ParseDestination(fmt.Sprintf("%s:%s", network, addr))
			if err != nil {
				return nil, err
			}
			return v2core.Dial(ctx, inst, dest)
		},
	}

	client := &http.Client{
		Transport: tr,
		Timeout:   12 * time.Second,
	}

	if len(url) == 0 {
		url = "https://www.google.com/generate_204"
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return -1, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return -1, fmt.Errorf("unexpected status code: %s", resp.Status)
	}
	return time.Since(start).Milliseconds(), nil
}

// ... (rest of the code)
