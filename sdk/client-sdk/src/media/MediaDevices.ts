/**
 * Media device management utilities
 */

/** Device info */
export interface DeviceInfo {
  deviceId: string;
  label: string;
  kind: MediaDeviceKind;
}

/** Media constraints */
export interface MediaConstraints {
  video?: boolean | MediaTrackConstraints;
  audio?: boolean | MediaTrackConstraints;
}

/** Screen share options */
export interface ScreenShareOptions {
  video?: boolean | MediaTrackConstraints;
  audio?: boolean | MediaTrackConstraints;
  preferCurrentTab?: boolean;
  selfBrowserSurface?: 'include' | 'exclude';
  surfaceSwitching?: 'include' | 'exclude';
  systemAudio?: 'include' | 'exclude';
}

/**
 * Media device manager
 */
export class MediaDevices {
  /**
   * Get list of available media devices
   */
  public static async getDevices(): Promise<DeviceInfo[]> {
    const devices = await navigator.mediaDevices.enumerateDevices();
    return devices.map((device) => ({
      deviceId: device.deviceId,
      label: device.label,
      kind: device.kind,
    }));
  }

  /**
   * Get available video input devices (cameras)
   */
  public static async getCameras(): Promise<DeviceInfo[]> {
    const devices = await this.getDevices();
    return devices.filter((d) => d.kind === 'videoinput');
  }

  /**
   * Get available audio input devices (microphones)
   */
  public static async getMicrophones(): Promise<DeviceInfo[]> {
    const devices = await this.getDevices();
    return devices.filter((d) => d.kind === 'audioinput');
  }

  /**
   * Get available audio output devices (speakers)
   */
  public static async getSpeakers(): Promise<DeviceInfo[]> {
    const devices = await this.getDevices();
    return devices.filter((d) => d.kind === 'audiooutput');
  }

  /**
   * Get user media (camera and/or microphone)
   */
  public static async getUserMedia(
    constraints: MediaConstraints = { video: true, audio: true }
  ): Promise<MediaStream> {
    return navigator.mediaDevices.getUserMedia(constraints);
  }

  /**
   * Get display media (screen share)
   */
  public static async getDisplayMedia(
    options: ScreenShareOptions = { video: true }
  ): Promise<MediaStream> {
    return navigator.mediaDevices.getDisplayMedia(options as DisplayMediaStreamOptions);
  }

  /**
   * Check if getUserMedia is supported
   */
  public static isGetUserMediaSupported(): boolean {
    return (
      typeof navigator !== 'undefined' &&
      navigator.mediaDevices !== undefined &&
      typeof navigator.mediaDevices.getUserMedia === 'function'
    );
  }

  /**
   * Check if getDisplayMedia is supported
   */
  public static isGetDisplayMediaSupported(): boolean {
    return (
      typeof navigator !== 'undefined' &&
      navigator.mediaDevices !== undefined &&
      typeof navigator.mediaDevices.getDisplayMedia === 'function'
    );
  }

  /**
   * Request camera and microphone permissions
   */
  public static async requestPermissions(
    constraints: MediaConstraints = { video: true, audio: true }
  ): Promise<PermissionStatus[]> {
    const results: PermissionStatus[] = [];

    if (constraints.video !== undefined && constraints.video !== false) {
      try {
        const status = await navigator.permissions.query({
          name: 'camera' as PermissionName,
        });
        results.push(status);
      } catch {
        // Permission API might not support camera
      }
    }

    if (constraints.audio !== undefined && constraints.audio !== false) {
      try {
        const status = await navigator.permissions.query({
          name: 'microphone' as PermissionName,
        });
        results.push(status);
      } catch {
        // Permission API might not support microphone
      }
    }

    return results;
  }

  /**
   * Listen for device changes
   * Returns a cleanup function, or a no-op if not supported (SSR/non-browser)
   */
  public static onDeviceChange(
    callback: (devices: DeviceInfo[]) => void
  ): () => void {
    // Check for SSR/non-browser environment
    if (
      typeof navigator === 'undefined' ||
      navigator.mediaDevices === undefined
    ) {
      // Return no-op cleanup function for SSR compatibility
      return () => {
        // No cleanup needed
      };
    }

    const handler = async (): Promise<void> => {
      const devices = await this.getDevices();
      callback(devices);
    };

    navigator.mediaDevices.addEventListener('devicechange', handler);

    // Return cleanup function
    return () => {
      navigator.mediaDevices.removeEventListener('devicechange', handler);
    };
  }

  /**
   * Create video constraints for a specific resolution
   */
  public static createVideoConstraints(options: {
    width?: number;
    height?: number;
    frameRate?: number;
    deviceId?: string;
    facingMode?: 'user' | 'environment';
  }): MediaTrackConstraints {
    const constraints: MediaTrackConstraints = {};

    if (options.width !== undefined) {
      constraints.width = { ideal: options.width };
    }

    if (options.height !== undefined) {
      constraints.height = { ideal: options.height };
    }

    if (options.frameRate !== undefined) {
      constraints.frameRate = { ideal: options.frameRate };
    }

    if (options.deviceId !== undefined) {
      constraints.deviceId = { exact: options.deviceId };
    }

    if (options.facingMode !== undefined) {
      constraints.facingMode = { ideal: options.facingMode };
    }

    return constraints;
  }

  /**
   * Create audio constraints
   */
  public static createAudioConstraints(options: {
    deviceId?: string;
    echoCancellation?: boolean;
    noiseSuppression?: boolean;
    autoGainControl?: boolean;
  }): MediaTrackConstraints {
    const constraints: MediaTrackConstraints = {};

    if (options.deviceId !== undefined) {
      constraints.deviceId = { exact: options.deviceId };
    }

    if (options.echoCancellation !== undefined) {
      constraints.echoCancellation = options.echoCancellation;
    }

    if (options.noiseSuppression !== undefined) {
      constraints.noiseSuppression = options.noiseSuppression;
    }

    if (options.autoGainControl !== undefined) {
      constraints.autoGainControl = options.autoGainControl;
    }

    return constraints;
  }
}
