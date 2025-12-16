package vmware

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/nirarg/vm-deep-inspection-demo/internal/config"
	"github.com/sirupsen/logrus"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/session/cache"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"
)

// Client represents a VMware vSphere client with connection management
type Client struct {
	config     config.VMwareConfig
	logger     *logrus.Logger
	client     *govmomi.Client
	session    *cache.Session
	mutex      sync.RWMutex
	isLoggedIn bool
}

// NewClient creates a new VMware client instance
func NewClient(cfg config.VMwareConfig, logger *logrus.Logger) *Client {
	return &Client{
		config: cfg,
		logger: logger,
	}
}

// Connect establishes a connection to vSphere with session management
func (c *Client) Connect(ctx context.Context) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Parse vCenter URL
	vcenterURL, err := url.Parse(c.config.VCenterURL)
	if err != nil {
		return fmt.Errorf("invalid vCenter URL: %w", err)
	}

	// Set credentials in URL
	vcenterURL.User = url.UserPassword(c.config.Username, c.config.Password)

	c.logger.WithFields(logrus.Fields{
		"vcenter_url": c.config.VCenterURL,
	}).Info("Connecting to vCenter")

	// Create context with timeout for connection
	connectCtx, cancel := context.WithTimeout(ctx, c.config.ConnectionTimeout)
	defer cancel()

	// Configure TLS settings
	soapClient := soap.NewClient(vcenterURL, c.config.InsecureSkipVerify)
	if c.config.InsecureSkipVerify {
		soapClient.DefaultTransport().TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	// Set request timeout
	soapClient.Timeout = c.config.RequestTimeout

	// Create vim25 client
	vimClient, err := vim25.NewClient(connectCtx, soapClient)
	if err != nil {
		return fmt.Errorf("failed to create vim25 client: %w", err)
	}

	// Create govmomi client
	c.client = &govmomi.Client{
		Client:         vimClient,
		SessionManager: session.NewManager(vimClient),
	}

	// Create session WITHOUT disk caching to avoid stale sessions
	// Setting an environment variable to disable cache file
	os.Setenv("GOVMOMI_SESSION_CACHE_DISABLE", "true")

	c.session = &cache.Session{
		URL:      vcenterURL,
		Insecure: c.config.InsecureSkipVerify,
		DirSOAP:  "", // Empty = no cache file on disk
	}

	// Login with retry logic
	if err := c.loginWithRetry(connectCtx); err != nil {
		c.logger.WithFields(logrus.Fields{
			"vcenter_url": c.config.VCenterURL,
			"error":       err,
		}).Error("Failed to login to vCenter after retries")
		return fmt.Errorf("failed to login to vCenter: %w", err)
	}

	// Verify the session is active by getting user session
	sessionMgr := session.NewManager(c.client.Client)
	userSession, err := sessionMgr.UserSession(connectCtx)
	if err != nil {
		c.logger.WithError(err).Error("Failed to verify user session after login")
		return fmt.Errorf("failed to verify session: %w", err)
	}

	c.isLoggedIn = true
	c.logger.WithFields(logrus.Fields{
		"user":     userSession.UserName,
		"session":  userSession.Key,
		"login_at": userSession.LoginTime,
	}).Info("Successfully connected and authenticated to vCenter")
	return nil
}

// loginWithRetry attempts to login with retry logic
func (c *Client) loginWithRetry(ctx context.Context) error {
	var lastErr error

	for attempt := 0; attempt <= c.config.RetryAttempts; attempt++ {
		if attempt > 0 {
			c.logger.WithFields(logrus.Fields{
				"attempt": attempt,
				"delay":   c.config.RetryDelay,
			}).Warn("Retrying vCenter login")

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(c.config.RetryDelay):
			}
		}

		// Attempt login - cache.Session.Login will NOT use disk cache since DirSOAP is empty
		err := c.session.Login(ctx, c.client.Client, nil)
		if err == nil {
			c.logger.WithField("attempt", attempt+1).Info("Login successful")
			return nil
		}

		lastErr = err
		c.logger.WithFields(logrus.Fields{
			"attempt": attempt + 1,
			"error":   err,
		}).Warn("Login attempt failed")
	}

	return fmt.Errorf("login failed after %d attempts: %w", c.config.RetryAttempts+1, lastErr)
}

// Disconnect closes the connection to vSphere
func (c *Client) Disconnect(ctx context.Context) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.client == nil || !c.isLoggedIn {
		return nil
	}

	c.logger.Info("Disconnecting from vCenter")

	// Logout with timeout
	logoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := c.session.Logout(logoutCtx, c.client.Client); err != nil {
		c.logger.WithError(err).Warn("Error during logout")
		// Don't return error as we want to cleanup anyway
	}

	c.isLoggedIn = false
	c.client = nil
	c.logger.Info("Disconnected from vCenter")
	return nil
}

// IsConnected returns true if the client is connected and logged in
func (c *Client) IsConnected() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.client != nil && c.isLoggedIn
}

// GetClient returns the underlying govmomi client
// This method ensures the connection is active before returning
func (c *Client) GetClient(ctx context.Context) (*govmomi.Client, error) {
	c.mutex.RLock()
	if c.client != nil && c.isLoggedIn {
		client := c.client
		c.mutex.RUnlock()

		// Verify the session is still valid by doing a quick health check
		sessionMgr := session.NewManager(client.Client)
		if _, err := sessionMgr.UserSession(ctx); err != nil {
			c.logger.WithError(err).Warn("Session validation failed, reconnecting")
			c.mutex.RUnlock()

			// Session is invalid, reconnect
			if err := c.Reconnect(ctx); err != nil {
				return nil, fmt.Errorf("failed to reconnect after session validation failure: %w", err)
			}

			c.mutex.RLock()
			defer c.mutex.RUnlock()
			return c.client, nil
		}

		return client, nil
	}
	c.mutex.RUnlock()

	// If not connected, attempt to connect
	c.logger.Info("Client not connected, attempting to connect")
	if err := c.Connect(ctx); err != nil {
		c.logger.WithError(err).Error("Failed to establish connection")
		return nil, fmt.Errorf("failed to establish connection: %w", err)
	}

	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.client, nil
}

// Reconnect forces a reconnection to vSphere
func (c *Client) Reconnect(ctx context.Context) error {
	c.logger.Info("Forcing reconnection to vCenter")

	// Disconnect first (ignore errors)
	_ = c.Disconnect(ctx)

	// Connect again
	return c.Connect(ctx)
}

// HealthCheck verifies the connection is still valid
func (c *Client) HealthCheck(ctx context.Context) error {
	client, err := c.GetClient(ctx)
	if err != nil {
		return fmt.Errorf("connection not available: %w", err)
	}

	// Create a context with short timeout for health check
	healthCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Try to get the current session from vCenter as a simple health check
	sessionMgr := session.NewManager(client.Client)
	_, err = sessionMgr.UserSession(healthCtx)
	if err != nil {
		c.logger.WithError(err).Warn("Health check failed, attempting reconnection")
		return c.Reconnect(ctx)
	}

	return nil
}

// GetConfig returns the VMware configuration
func (c *Client) GetConfig() config.VMwareConfig {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.config
}

// GetVCenterURL returns the vCenter URL
func (c *Client) GetVCenterURL() string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.config.VCenterURL
}

// GetCredentials returns the vCenter username and password
func (c *Client) GetCredentials() (string, string) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.config.Username, c.config.Password
}