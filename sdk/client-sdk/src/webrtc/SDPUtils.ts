/**
 * SDP manipulation utilities
 */

/** Codec info extracted from SDP */
export interface CodecInfo {
  payloadType: number;
  codec: string;
  clockRate: number;
  channels?: number;
  fmtp?: string;
}

/** Media section info */
export interface MediaSection {
  type: 'audio' | 'video';
  mid: string;
  direction: 'sendonly' | 'recvonly' | 'sendrecv' | 'inactive';
  codecs: CodecInfo[];
}

/**
 * SDP utility functions
 */
export class SDPUtils {
  /**
   * Parse SDP to extract media sections
   */
  public static parseMediaSections(sdp: string): MediaSection[] {
    const sections: MediaSection[] = [];
    const lines = sdp.split('\r\n');

    let currentMedia: Partial<MediaSection> | null = null;
    let currentCodecs: CodecInfo[] = [];

    for (const line of lines) {
      if (line.startsWith('m=')) {
        // Save previous section
        if (currentMedia !== null) {
          sections.push({
            type: currentMedia.type as 'audio' | 'video',
            mid: currentMedia.mid ?? '',
            direction: currentMedia.direction ?? 'sendrecv',
            codecs: currentCodecs,
          });
        }

        // Start new section
        const match = /^m=(audio|video)/.exec(line);
        currentMedia = {
          type: match?.[1] as 'audio' | 'video',
        };
        currentCodecs = [];
      } else if (currentMedia !== null) {
        if (line.startsWith('a=mid:')) {
          currentMedia.mid = line.slice(6);
        } else if (line.startsWith('a=sendonly')) {
          currentMedia.direction = 'sendonly';
        } else if (line.startsWith('a=recvonly')) {
          currentMedia.direction = 'recvonly';
        } else if (line.startsWith('a=sendrecv')) {
          currentMedia.direction = 'sendrecv';
        } else if (line.startsWith('a=inactive')) {
          currentMedia.direction = 'inactive';
        } else if (line.startsWith('a=rtpmap:')) {
          const codec = this.parseRtpmap(line);
          if (codec !== null) {
            currentCodecs.push(codec);
          }
        }
      }
    }

    // Don't forget last section
    if (currentMedia !== null) {
      sections.push({
        type: currentMedia.type as 'audio' | 'video',
        mid: currentMedia.mid ?? '',
        direction: currentMedia.direction ?? 'sendrecv',
        codecs: currentCodecs,
      });
    }

    return sections;
  }

  /**
   * Parse rtpmap line
   */
  private static parseRtpmap(line: string): CodecInfo | null {
    // a=rtpmap:96 VP8/90000
    const match = /^a=rtpmap:(\d+)\s+([^/]+)\/(\d+)(?:\/(\d+))?/.exec(line);
    if (match === null) {
      return null;
    }

    return {
      payloadType: parseInt(match[1] ?? '0', 10),
      codec: match[2] ?? '',
      clockRate: parseInt(match[3] ?? '0', 10),
      channels: match[4] !== undefined ? parseInt(match[4], 10) : undefined,
    };
  }

  /**
   * Set codec preference in SDP
   */
  public static setCodecPreference(
    sdp: string,
    mediaType: 'audio' | 'video',
    preferredCodecs: string[]
  ): string {
    const lines = sdp.split('\r\n');
    const result: string[] = [];
    let inMedia = false;
    let currentMediaType: string | null = null;
    const payloadTypes: Map<string, number> = new Map();

    // First pass: collect payload types for codecs
    for (const line of lines) {
      if (line.startsWith('m=')) {
        const match = /^m=(audio|video)/.exec(line);
        currentMediaType = match?.[1] ?? null;
        inMedia = currentMediaType === mediaType;
      }

      if (inMedia && line.startsWith('a=rtpmap:')) {
        const codec = this.parseRtpmap(line);
        if (codec !== null) {
          payloadTypes.set(codec.codec.toLowerCase(), codec.payloadType);
        }
      }
    }

    // Second pass: reorder payload types in m= line
    inMedia = false;
    currentMediaType = null;

    for (const line of lines) {
      if (line.startsWith('m=')) {
        const match = /^m=(audio|video)/.exec(line);
        currentMediaType = match?.[1] ?? null;
        inMedia = currentMediaType === mediaType;

        if (inMedia) {
          // Reorder payload types
          const parts = line.split(' ');
          if (parts.length >= 4) {
            const header = parts.slice(0, 3);
            const currentPayloads = parts.slice(3);

            // Get preferred payload types in order
            const orderedPayloads: string[] = [];
            for (const codec of preferredCodecs) {
              const pt = payloadTypes.get(codec.toLowerCase());
              if (pt !== undefined) {
                const ptStr = pt.toString();
                if (currentPayloads.includes(ptStr)) {
                  orderedPayloads.push(ptStr);
                }
              }
            }

            // Add remaining payload types
            for (const pt of currentPayloads) {
              if (!orderedPayloads.includes(pt)) {
                orderedPayloads.push(pt);
              }
            }

            result.push([...header, ...orderedPayloads].join(' '));
            continue;
          }
        }
      }

      result.push(line);
    }

    return result.join('\r\n');
  }

  /**
   * Get ICE credentials from SDP
   */
  public static getICECredentials(sdp: string): {
    ufrag: string;
    pwd: string;
  } | null {
    const ufragMatch = /a=ice-ufrag:(.+)/.exec(sdp);
    const pwdMatch = /a=ice-pwd:(.+)/.exec(sdp);

    if (ufragMatch !== null && pwdMatch !== null) {
      return {
        ufrag: ufragMatch[1] ?? '',
        pwd: pwdMatch[1] ?? '',
      };
    }

    return null;
  }

  /**
   * Get fingerprint from SDP
   */
  public static getFingerprint(sdp: string): {
    algorithm: string;
    value: string;
  } | null {
    const match = /a=fingerprint:(\S+)\s+(.+)/.exec(sdp);
    if (match !== null) {
      return {
        algorithm: match[1] ?? '',
        value: match[2] ?? '',
      };
    }
    return null;
  }

  /**
   * Check if SDP uses Unified Plan
   */
  public static isUnifiedPlan(sdp: string): boolean {
    // Unified Plan SDPs have mid attributes in each m= section
    const mLines = sdp.match(/^m=/gm)?.length ?? 0;
    const midLines = sdp.match(/^a=mid:/gm)?.length ?? 0;
    return mLines > 0 && mLines === midLines;
  }

  /**
   * Extract simulcast layers from SDP
   */
  public static extractSimulcastLayers(sdp: string): string[] {
    const layers: string[] = [];

    // Look for rid lines
    const ridMatches = sdp.matchAll(/a=rid:(\w+)\s+send/g);
    for (const match of ridMatches) {
      const rid = match[1];
      if (rid !== undefined) {
        layers.push(rid);
      }
    }

    return layers;
  }

  /**
   * Add simulcast to SDP
   */
  public static addSimulcast(
    sdp: string,
    layers: string[] = ['h', 'm', 'l']
  ): string {
    const lines = sdp.split('\r\n');
    const result: string[] = [];
    let inVideo = false;
    let addedSimulcast = false;

    for (let i = 0; i < lines.length; i++) {
      const line = lines[i] ?? '';
      result.push(line);

      if (line.startsWith('m=video')) {
        inVideo = true;
        addedSimulcast = false;
      } else if (line.startsWith('m=')) {
        inVideo = false;
      }

      // Add simulcast attributes after the last a= line in video section
      if (
        inVideo &&
        !addedSimulcast &&
        line.startsWith('a=') &&
        (i + 1 >= lines.length ||
          !lines[i + 1]?.startsWith('a=') ||
          lines[i + 1]?.startsWith('m='))
      ) {
        // Add rid lines
        for (const layer of layers) {
          result.push(`a=rid:${layer} send`);
        }

        // Add simulcast line
        result.push(`a=simulcast:send ${layers.join(';')}`);
        addedSimulcast = true;
      }
    }

    return result.join('\r\n');
  }
}
