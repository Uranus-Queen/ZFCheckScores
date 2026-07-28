/**
 * 浏览器端 AES-256-GCM 解密，与 Go 侧 internal/push/encrypt.go 严格互逆：
 *   key   = SHA-256(密钥字符串)
 *   载荷  = base64( nonce(12B) || GCM 密文 )
 * 密钥只存在于 URL # 片段（不上服务器、不进仓库、不进 referrer）。
 */

function b64ToBytes(b64: string): Uint8Array {
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

export async function decryptPayload(ct: string, key: string): Promise<string> {
  const keyBytes = await crypto.subtle.digest(
    "SHA-256",
    new TextEncoder().encode(key),
  );
  const aesKey = await crypto.subtle.importKey(
    "raw",
    keyBytes,
    "AES-GCM",
    false,
    ["decrypt"],
  );
  const raw = b64ToBytes(ct);
  const nonce = raw.slice(0, 12);
  const data = raw.slice(12);
  const plain = await crypto.subtle.decrypt(
    { name: "AES-GCM", iv: nonce },
    aesKey,
    data,
  );
  return new TextDecoder().decode(plain);
}
