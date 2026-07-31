// Source-reading shared by the verification scripts.
//
// Comments describe violations as often as they commit them: check-token-coverage's
// own header names a hex and an arbitrary value, and a file that stopped using a
// primitive often keeps the import in a comment. Both checks read source looking
// for exactly the thing a comment is most likely to be talking about, so both read
// it stripped.
export function stripComments(source) {
  return source
    .replaceAll(/<!--[\s\S]*?-->/g, '')
    .replaceAll(/\/\*[\s\S]*?\*\//g, '')
    .replaceAll(/(^|[\s;{(])\/\/[^\n]*/g, '$1');
}
