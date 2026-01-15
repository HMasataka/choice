package simulcast

import (
	"context"
	"fmt"
	"sync"

	"github.com/HMasataka/choice/internal/media"
)

// Controller manages simulcast layer selection for subscribers.
// Per design.md section 3.5.2: SimulcastController interface.
type Controller interface {
	// SetPreferredLayer sets the subscriber's preferred layer.
	// This is called when a client explicitly requests a layer via setPreferredLayer.
	SetPreferredLayer(ctx context.Context, subscriptionID media.SubscriptionID, layer media.SimulcastLayer) error

	// GetCurrentLayer returns the actual layer being sent.
	GetCurrentLayer(subscriptionID media.SubscriptionID) (media.SimulcastLayer, error)

	// GetPreferredLayer returns the client's preferred layer (may differ from actual).
	GetPreferredLayer(subscriptionID media.SubscriptionID) (media.SimulcastLayer, error)

	// OnBandwidthEstimate handles bandwidth updates from TWCC/REMB.
	// Returns layer change result if the actual layer changed.
	OnBandwidthEstimate(ctx context.Context, subscriberID string, bps uint64) []LayerChangeResult

	// OnPacketLoss handles packet loss detection.
	// Returns layer change result if the actual layer changed.
	OnPacketLoss(ctx context.Context, subscriberID string, lossRate float64) []LayerChangeResult

	// RegisterSubscription registers a subscription with its available layers.
	// availableLayers should be the layers supported by the source track.
	RegisterSubscription(subscriptionID media.SubscriptionID, subscriberID string, trackID media.TrackID, availableLayers []media.SimulcastLayer, preferredLayer media.SimulcastLayer) error

	// UnregisterSubscription removes a subscription from the controller.
	UnregisterSubscription(subscriptionID media.SubscriptionID) error

	// GetSubscriptionState returns the current state of a subscription (for debugging/testing).
	GetSubscriptionState(subscriptionID media.SubscriptionID) (*SubscriptionState, error)
}

// SubscriptionState holds the state for a single subscription.
type SubscriptionState struct {
	SubscriptionID  media.SubscriptionID
	SubscriberID    string
	TrackID         media.TrackID
	AvailableLayers []media.SimulcastLayer
	PreferredLayer  media.SimulcastLayer
	ActualLayer     media.SimulcastLayer
}

// Copy creates a deep copy of SubscriptionState.
func (s *SubscriptionState) Copy() *SubscriptionState {
	if s == nil {
		return nil
	}
	layers := make([]media.SimulcastLayer, len(s.AvailableLayers))
	copy(layers, s.AvailableLayers)
	return &SubscriptionState{
		SubscriptionID:  s.SubscriptionID,
		SubscriberID:    s.SubscriberID,
		TrackID:         s.TrackID,
		AvailableLayers: layers,
		PreferredLayer:  s.PreferredLayer,
		ActualLayer:     s.ActualLayer,
	}
}

// LayerChangeResult contains information about a layer change.
type LayerChangeResult struct {
	SubscriptionID media.SubscriptionID
	Changed        bool
	PreviousLayer  media.SimulcastLayer
	CurrentLayer   media.SimulcastLayer
	Reason         LayerChangeReason
}

// LayerChangeReason indicates why a layer change occurred.
type LayerChangeReason string

const (
	// LayerChangeReasonRequested indicates the layer changed due to client request.
	LayerChangeReasonRequested LayerChangeReason = "requested"
	// LayerChangeReasonBandwidth indicates the layer changed due to bandwidth constraints.
	LayerChangeReasonBandwidth LayerChangeReason = "bandwidth"
	// LayerChangeReasonPacketLoss indicates the layer changed due to packet loss.
	LayerChangeReasonPacketLoss LayerChangeReason = "packet_loss"
	// LayerChangeReasonUnavailable indicates the requested layer is not available.
	LayerChangeReasonUnavailable LayerChangeReason = "unavailable"
	// LayerChangeReasonRecovery indicates the layer changed due to quality recovery.
	LayerChangeReasonRecovery LayerChangeReason = "recovery"
)

// controller is the concrete implementation of Controller.
type controller struct {
	mu sync.RWMutex

	// subscriptions maps subscriptionID to its state
	subscriptions map[media.SubscriptionID]*SubscriptionState

	// bySubscriber maps subscriberID to a set of subscriptionIDs
	// for efficient lookup when processing bandwidth/packet loss events
	bySubscriber map[string]map[media.SubscriptionID]struct{}

	// subscriberBandwidth tracks the last known bandwidth for each subscriber.
	// This is used by OnPacketLoss to gate recovery decisions.
	// Per design.md: "<1% packet loss with sufficient bandwidth allows recovery"
	subscriberBandwidth map[string]uint64

	// layerSelector handles the actual layer selection logic
	layerSelector *LayerSelector

	// callback for layer changes (optional)
	onLayerChange func(result LayerChangeResult)
}

// ControllerConfig contains configuration for the controller.
type ControllerConfig struct {
	// OnLayerChange is called when a layer change occurs.
	OnLayerChange func(result LayerChangeResult)

	// LayerSelectorConfig contains configuration for layer selection logic.
	LayerSelectorConfig *LayerSelectorConfig
}

// DefaultControllerConfig returns the default controller configuration.
func DefaultControllerConfig() *ControllerConfig {
	return &ControllerConfig{
		LayerSelectorConfig: DefaultLayerSelectorConfig(),
	}
}

// NewController creates a new simulcast controller.
func NewController(cfg *ControllerConfig) Controller {
	if cfg == nil {
		cfg = DefaultControllerConfig()
	}
	return &controller{
		subscriptions:       make(map[media.SubscriptionID]*SubscriptionState),
		bySubscriber:        make(map[string]map[media.SubscriptionID]struct{}),
		subscriberBandwidth: make(map[string]uint64),
		layerSelector:       NewLayerSelector(cfg.LayerSelectorConfig),
		onLayerChange:       cfg.OnLayerChange,
	}
}

// SetPreferredLayer sets the subscriber's preferred layer.
func (c *controller) SetPreferredLayer(ctx context.Context, subscriptionID media.SubscriptionID, layer media.SimulcastLayer) error {
	if err := subscriptionID.Validate(); err != nil {
		return fmt.Errorf("invalid subscription ID: %w", err)
	}
	if err := layer.Validate(); err != nil {
		return fmt.Errorf("invalid layer: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	state, exists := c.subscriptions[subscriptionID]
	if !exists {
		return fmt.Errorf("subscription %s not found", subscriptionID)
	}

	// Store previous layer for comparison
	previousLayer := state.ActualLayer
	state.PreferredLayer = layer

	// Select the best available layer based on preference
	actualLayer := c.layerSelector.SelectLayer(state.AvailableLayers, layer)
	state.ActualLayer = actualLayer

	// Emit change if layer actually changed
	if previousLayer != actualLayer && c.onLayerChange != nil {
		reason := LayerChangeReasonRequested
		if actualLayer != layer {
			reason = LayerChangeReasonUnavailable
		}
		c.onLayerChange(LayerChangeResult{
			SubscriptionID: subscriptionID,
			Changed:        true,
			PreviousLayer:  previousLayer,
			CurrentLayer:   actualLayer,
			Reason:         reason,
		})
	}

	return nil
}

// GetCurrentLayer returns the actual layer being sent.
func (c *controller) GetCurrentLayer(subscriptionID media.SubscriptionID) (media.SimulcastLayer, error) {
	if err := subscriptionID.Validate(); err != nil {
		return "", fmt.Errorf("invalid subscription ID: %w", err)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	state, exists := c.subscriptions[subscriptionID]
	if !exists {
		return "", fmt.Errorf("subscription %s not found", subscriptionID)
	}

	return state.ActualLayer, nil
}

// GetPreferredLayer returns the client's preferred layer.
func (c *controller) GetPreferredLayer(subscriptionID media.SubscriptionID) (media.SimulcastLayer, error) {
	if err := subscriptionID.Validate(); err != nil {
		return "", fmt.Errorf("invalid subscription ID: %w", err)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	state, exists := c.subscriptions[subscriptionID]
	if !exists {
		return "", fmt.Errorf("subscription %s not found", subscriptionID)
	}

	return state.PreferredLayer, nil
}

// OnBandwidthEstimate handles bandwidth updates from TWCC/REMB.
func (c *controller) OnBandwidthEstimate(ctx context.Context, subscriberID string, bps uint64) []LayerChangeResult {
	if subscriberID == "" {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Store bandwidth for use in OnPacketLoss recovery decisions
	c.subscriberBandwidth[subscriberID] = bps

	subIDs, exists := c.bySubscriber[subscriberID]
	if !exists || len(subIDs) == 0 {
		return nil
	}

	var results []LayerChangeResult
	for subID := range subIDs {
		state, ok := c.subscriptions[subID]
		if !ok {
			continue
		}

		previousLayer := state.ActualLayer
		newLayer := c.layerSelector.SelectLayerForBandwidth(state.AvailableLayers, state.PreferredLayer, bps)

		if newLayer != previousLayer {
			state.ActualLayer = newLayer
			result := LayerChangeResult{
				SubscriptionID: subID,
				Changed:        true,
				PreviousLayer:  previousLayer,
				CurrentLayer:   newLayer,
				Reason:         LayerChangeReasonBandwidth,
			}
			results = append(results, result)

			if c.onLayerChange != nil {
				c.onLayerChange(result)
			}
		}
	}

	return results
}

// OnPacketLoss handles packet loss detection.
// Uses the last known bandwidth estimate to gate recovery decisions.
// Per design.md: "<1% packet loss with sufficient bandwidth allows recovery"
func (c *controller) OnPacketLoss(ctx context.Context, subscriberID string, lossRate float64) []LayerChangeResult {
	if subscriberID == "" {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	subIDs, exists := c.bySubscriber[subscriberID]
	if !exists || len(subIDs) == 0 {
		return nil
	}

	// Get the last known bandwidth for this subscriber
	bps := c.subscriberBandwidth[subscriberID]

	var results []LayerChangeResult
	for subID := range subIDs {
		state, ok := c.subscriptions[subID]
		if !ok {
			continue
		}

		previousLayer := state.ActualLayer
		newLayer := c.layerSelector.SelectLayerForPacketLoss(state.AvailableLayers, state.PreferredLayer, state.ActualLayer, lossRate, bps)

		if newLayer != previousLayer {
			state.ActualLayer = newLayer
			reason := LayerChangeReasonPacketLoss
			// If moving to a higher layer, it's recovery
			if layerPriority(newLayer) > layerPriority(previousLayer) {
				reason = LayerChangeReasonRecovery
			}
			result := LayerChangeResult{
				SubscriptionID: subID,
				Changed:        true,
				PreviousLayer:  previousLayer,
				CurrentLayer:   newLayer,
				Reason:         reason,
			}
			results = append(results, result)

			if c.onLayerChange != nil {
				c.onLayerChange(result)
			}
		}
	}

	return results
}

// RegisterSubscription registers a subscription with its available layers.
func (c *controller) RegisterSubscription(subscriptionID media.SubscriptionID, subscriberID string, trackID media.TrackID, availableLayers []media.SimulcastLayer, preferredLayer media.SimulcastLayer) error {
	if err := subscriptionID.Validate(); err != nil {
		return fmt.Errorf("invalid subscription ID: %w", err)
	}
	if subscriberID == "" {
		return fmt.Errorf("subscriber ID cannot be empty")
	}
	if err := trackID.Validate(); err != nil {
		return fmt.Errorf("invalid track ID: %w", err)
	}
	if len(availableLayers) == 0 {
		return fmt.Errorf("available layers cannot be empty")
	}
	// Validate each layer
	for _, layer := range availableLayers {
		if err := layer.Validate(); err != nil {
			return fmt.Errorf("invalid available layer: %w", err)
		}
	}
	if err := preferredLayer.Validate(); err != nil {
		return fmt.Errorf("invalid preferred layer: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check for duplicate registration
	if _, exists := c.subscriptions[subscriptionID]; exists {
		return fmt.Errorf("subscription %s already registered", subscriptionID)
	}

	// Determine actual layer based on preference and availability
	actualLayer := c.layerSelector.SelectLayer(availableLayers, preferredLayer)

	// Create subscription state
	layers := make([]media.SimulcastLayer, len(availableLayers))
	copy(layers, availableLayers)
	state := &SubscriptionState{
		SubscriptionID:  subscriptionID,
		SubscriberID:    subscriberID,
		TrackID:         trackID,
		AvailableLayers: layers,
		PreferredLayer:  preferredLayer,
		ActualLayer:     actualLayer,
	}

	// Store state
	c.subscriptions[subscriptionID] = state

	// Index by subscriber
	if c.bySubscriber[subscriberID] == nil {
		c.bySubscriber[subscriberID] = make(map[media.SubscriptionID]struct{})
	}
	c.bySubscriber[subscriberID][subscriptionID] = struct{}{}

	return nil
}

// UnregisterSubscription removes a subscription from the controller.
func (c *controller) UnregisterSubscription(subscriptionID media.SubscriptionID) error {
	if err := subscriptionID.Validate(); err != nil {
		return fmt.Errorf("invalid subscription ID: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	state, exists := c.subscriptions[subscriptionID]
	if !exists {
		return fmt.Errorf("subscription %s not found", subscriptionID)
	}

	// Remove from subscriber index
	if subSet, ok := c.bySubscriber[state.SubscriberID]; ok {
		delete(subSet, subscriptionID)
		if len(subSet) == 0 {
			delete(c.bySubscriber, state.SubscriberID)
			// Also clean up bandwidth tracking for this subscriber
			// to prevent memory leak on long-running servers
			delete(c.subscriberBandwidth, state.SubscriberID)
		}
	}

	// Remove from subscriptions
	delete(c.subscriptions, subscriptionID)

	return nil
}

// GetSubscriptionState returns the current state of a subscription.
func (c *controller) GetSubscriptionState(subscriptionID media.SubscriptionID) (*SubscriptionState, error) {
	if err := subscriptionID.Validate(); err != nil {
		return nil, fmt.Errorf("invalid subscription ID: %w", err)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	state, exists := c.subscriptions[subscriptionID]
	if !exists {
		return nil, fmt.Errorf("subscription %s not found", subscriptionID)
	}

	return state.Copy(), nil
}

// layerPriority returns the priority of a layer (higher is better).
func layerPriority(layer media.SimulcastLayer) int {
	switch layer {
	case media.SimulcastLayerHigh:
		return 3
	case media.SimulcastLayerMedium:
		return 2
	case media.SimulcastLayerLow:
		return 1
	default:
		return 0
	}
}
