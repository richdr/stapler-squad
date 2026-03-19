/**
 * Tests for TerminalDimensionCache - localStorage dimension persistence.
 */

import { getCachedDimensions, saveDimensions } from '../TerminalDimensionCache';

describe('TerminalDimensionCache', () => {
  let mockStorage: Record<string, string>;

  beforeEach(() => {
    mockStorage = {};

    jest.spyOn(Storage.prototype, 'getItem').mockImplementation((key: string) => {
      return mockStorage[key] ?? null;
    });
    jest.spyOn(Storage.prototype, 'setItem').mockImplementation((key: string, value: string) => {
      mockStorage[key] = value;
    });

    // Suppress console output in tests
    jest.spyOn(console, 'log').mockImplementation(() => {});
    jest.spyOn(console, 'warn').mockImplementation(() => {});
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  describe('saveDimensions', () => {
    it('should write to localStorage with correct key and value', () => {
      saveDimensions('session-123', 120, 40);

      expect(localStorage.setItem).toHaveBeenCalledWith(
        'terminal-dimensions-session-123',
        JSON.stringify({ cols: 120, rows: 40 })
      );
    });

    it('should overwrite existing cached dimensions', () => {
      saveDimensions('session-123', 80, 24);
      saveDimensions('session-123', 120, 40);

      const stored = JSON.parse(mockStorage['terminal-dimensions-session-123']);
      expect(stored).toEqual({ cols: 120, rows: 40 });
    });
  });

  describe('getCachedDimensions', () => {
    it('should read from localStorage and return dimensions', () => {
      mockStorage['terminal-dimensions-session-abc'] = JSON.stringify({ cols: 100, rows: 30 });

      const result = getCachedDimensions('session-abc');

      expect(result).toEqual({ cols: 100, rows: 30 });
    });

    it('should return null when no cached value exists', () => {
      const result = getCachedDimensions('nonexistent');

      expect(result).toBeNull();
    });

    it('should return null when localStorage contains invalid JSON', () => {
      mockStorage['terminal-dimensions-bad'] = 'not-json';

      const result = getCachedDimensions('bad');

      expect(result).toBeNull();
      expect(console.warn).toHaveBeenCalled();
    });
  });

  describe('error handling', () => {
    it('should handle localStorage.setItem throwing (quota exceeded)', () => {
      jest.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
        throw new DOMException('QuotaExceededError');
      });

      // Should not throw
      expect(() => saveDimensions('session-123', 80, 24)).not.toThrow();
      expect(console.warn).toHaveBeenCalled();
    });

    it('should handle localStorage.getItem throwing', () => {
      jest.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
        throw new Error('SecurityError');
      });

      const result = getCachedDimensions('session-123');

      expect(result).toBeNull();
      expect(console.warn).toHaveBeenCalled();
    });
  });
});
