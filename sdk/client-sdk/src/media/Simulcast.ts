/**
 * Simulcast configuration and utilities
 */

import type { SimulcastLayer } from '../utils/types';

/** Simulcast layer configuration */
export interface SimulcastLayerConfig {
  rid: SimulcastLayer;
  maxBitrate: number;
  maxFramerate: number;
  scaleResolutionDownBy: number;
}

/** Default simulcast configurations */
export const DEFAULT_SIMULCAST_LAYERS: SimulcastLayerConfig[] = [
  {
    rid: 'h',
    maxBitrate: 2_500_000, // 2.5 Mbps
    maxFramerate: 30,
    scaleResolutionDownBy: 1,
  },
  {
    rid: 'm',
    maxBitrate: 500_000, // 500 Kbps
    maxFramerate: 30,
    scaleResolutionDownBy: 2,
  },
  {
    rid: 'l',
    maxBitrate: 150_000, // 150 Kbps
    maxFramerate: 15,
    scaleResolutionDownBy: 4,
  },
];

/** Screen share simulcast layers */
export const SCREEN_SHARE_SIMULCAST_LAYERS: SimulcastLayerConfig[] = [
  {
    rid: 'h',
    maxBitrate: 3_000_000, // 3 Mbps
    maxFramerate: 15,
    scaleResolutionDownBy: 1,
  },
  {
    rid: 'm',
    maxBitrate: 1_000_000, // 1 Mbps
    maxFramerate: 10,
    scaleResolutionDownBy: 2,
  },
  {
    rid: 'l',
    maxBitrate: 400_000, // 400 Kbps
    maxFramerate: 5,
    scaleResolutionDownBy: 4,
  },
];

/**
 * Create RTCRtpEncodingParameters for simulcast
 */
export function createSimulcastEncodings(
  layers: SimulcastLayerConfig[] = DEFAULT_SIMULCAST_LAYERS
): RTCRtpEncodingParameters[] {
  return layers.map((layer) => ({
    rid: layer.rid,
    maxBitrate: layer.maxBitrate,
    maxFramerate: layer.maxFramerate,
    scaleResolutionDownBy: layer.scaleResolutionDownBy,
  }));
}

/**
 * Get layer configuration by rid
 */
export function getLayerConfig(
  rid: SimulcastLayer,
  layers: SimulcastLayerConfig[] = DEFAULT_SIMULCAST_LAYERS
): SimulcastLayerConfig | undefined {
  return layers.find((l) => l.rid === rid);
}

/**
 * Get next lower layer
 */
export function getNextLowerLayer(
  currentLayer: SimulcastLayer,
  availableLayers: SimulcastLayer[] = ['h', 'm', 'l']
): SimulcastLayer | null {
  const order: SimulcastLayer[] = ['h', 'm', 'l'];
  const currentIndex = order.indexOf(currentLayer);

  if (currentIndex === -1) {
    return null;
  }

  // Find next available lower layer
  for (let i = currentIndex + 1; i < order.length; i++) {
    const layer = order[i];
    if (layer !== undefined && availableLayers.includes(layer)) {
      return layer;
    }
  }

  return null;
}

/**
 * Get next higher layer
 */
export function getNextHigherLayer(
  currentLayer: SimulcastLayer,
  availableLayers: SimulcastLayer[] = ['h', 'm', 'l']
): SimulcastLayer | null {
  const order: SimulcastLayer[] = ['h', 'm', 'l'];
  const currentIndex = order.indexOf(currentLayer);

  if (currentIndex === -1) {
    return null;
  }

  // Find next available higher layer
  for (let i = currentIndex - 1; i >= 0; i--) {
    const layer = order[i];
    if (layer !== undefined && availableLayers.includes(layer)) {
      return layer;
    }
  }

  return null;
}

/**
 * Compare layers (returns negative if a < b, positive if a > b, 0 if equal)
 */
export function compareLayers(a: SimulcastLayer, b: SimulcastLayer): number {
  const order: SimulcastLayer[] = ['h', 'm', 'l'];
  return order.indexOf(a) - order.indexOf(b);
}

/**
 * Get best available layer not exceeding the requested layer
 */
export function getBestAvailableLayer(
  requestedLayer: SimulcastLayer,
  availableLayers: SimulcastLayer[]
): SimulcastLayer | null {
  const order: SimulcastLayer[] = ['h', 'm', 'l'];
  const requestedIndex = order.indexOf(requestedLayer);

  // Try requested layer first
  if (availableLayers.includes(requestedLayer)) {
    return requestedLayer;
  }

  // Find next lower available layer
  for (let i = requestedIndex + 1; i < order.length; i++) {
    const layer = order[i];
    if (layer !== undefined && availableLayers.includes(layer)) {
      return layer;
    }
  }

  // No lower layer available, return any available
  if (availableLayers.length > 0) {
    return availableLayers[0] ?? null;
  }

  return null;
}
