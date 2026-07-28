package push

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

// decrypterTemplate is a self-contained HTML page that holds ONLY the
// ciphertext (CT) and decrypts it in the browser. The decryption key travels
// in the URL fragment (#key), which is never sent to the server and never
// stored in the repo — so a public repository leaks nothing readable even
// though the file is committed. The browser derives the AES-256 key from the
// fragment via SHA-256, then AES-GCM decrypts and swaps the document in.
const decrypterTemplate = `<!DOCTYPE html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover"><title>成绩</title></head><body><div id="app" style="font-family:-apple-system,BlinkMacSystemFont,'PingFang SC','Microsoft YaHei',sans-serif;background:#000;color:#fff;min-height:100vh;display:flex;align-items:center;justify-content:center;padding:24px;text-align:center;line-height:1.6">正在解密成绩…</div><script>
const CT="%s";
function b64ToBytes(b){const bin=atob(b);const a=new Uint8Array(bin.length);for(let i=0;i<bin.length;i++)a[i]=bin.charCodeAt(i);return a;}
async function sha256(str){const d=await crypto.subtle.digest('SHA-256',new TextEncoder().encode(str));return new Uint8Array(d);}
async function main(){
  const key=location.hash.slice(1);
  const app=document.getElementById('app');
  if(!key){app.textContent='无访问密钥：请使用带 #密钥 的链接打开本页面。';return;}
  try{
    const keyBytes=await sha256(key);
    const imp=await crypto.subtle.importKey('raw',keyBytes,'AES-GCM',false,['decrypt']);
    const raw=b64ToBytes(CT);
    const nonceLen=12;
    const nonce=raw.slice(0,nonceLen);
    const data=raw.slice(nonceLen);
    const plainBuf=await crypto.subtle.decrypt({name:'AES-GCM',iv:nonce},imp,data);
    const html=new TextDecoder().decode(plainBuf);
    const doc=new DOMParser().parseFromString(html,'text/html');
    document.documentElement.innerHTML=doc.documentElement.innerHTML;
  }catch(e){
    app.textContent='解密失败：密钥错误或页面已失效。';
  }
}
main();
</script></body></html>`

// deriveKey expands a user-supplied key string into a 32-byte AES-256 key.
func deriveKey(keyStr string) []byte {
	sum := sha256.Sum256([]byte(keyStr))
	return sum[:]
}

// EncryptHTML encrypts plaintext with AES-256-GCM using a key derived from
// keyStr (SHA-256), returning base64(nonce || ciphertext). The output contains
// only base64 characters, so it is safe to embed in a page as a JS string
// constant (no quoting / escaping hazards).
func EncryptHTML(plaintext, keyStr string) (string, error) {
	block, err := aes.NewCipher(deriveKey(keyStr))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// EncryptAndWrap encrypts plaintext and returns a self-contained decrypter
// HTML page. The page holds only the ciphertext; the decryption key travels in
// the URL fragment (#key), which is never sent to the server and never stored
// in the repo — so a public repository leaks nothing readable.
func EncryptAndWrap(plaintext, keyStr string) (string, error) {
	ct, err := EncryptHTML(plaintext, keyStr)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(decrypterTemplate, ct), nil
}
