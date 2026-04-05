/**
 * JSON reviver that converts string-encoded integers back to JS numbers.
 * The backend serializes all uint64 fields as JSON strings (e.g. "3000000000")
 * to avoid JavaScript float64 precision loss.
 *
 * This reviver converts them back to number if within Number.MAX_SAFE_INTEGER,
 * otherwise keeps them as strings (display code must handle this).
 */
export function numericReviver(_key: string, value: unknown): unknown {
  if (typeof value === 'string' && /^\d+$/.test(value) && value.length > 0 && value.length <= 20) {
    const n = Number(value);
    if (Number.isSafeInteger(n)) return n;
    // Above MAX_SAFE_INTEGER — keep as string to avoid precision loss.
    return value;
  }
  return value;
}
