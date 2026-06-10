export function shellQuote(value: string) {
  return `'${value.replace(/'/g, `'\\''`)}'`;
}
