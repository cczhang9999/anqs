// CookieUtils.ts
export class CookieUtils {
  // 获取 cookie
  static get(name: string): string | null {
    const cookies = document.cookie.split(';')
    for (let i = 0; i < cookies.length; i++) {
      let cookie = cookies[i].trim()
      if (cookie.startsWith(name + '=')) {
        return decodeURIComponent(cookie.substring(name.length + 1))
      }
    }
    return null
  }

  // 设置 cookie
  static set(name: string, value: string, days: number = 7): void {
    const expires = new Date()
    expires.setTime(expires.getTime() + days * 24 * 60 * 60 * 1000) // 设置过期时间
    document.cookie = `${name}=${encodeURIComponent(value)};expires=${expires.toUTCString()};path=/`
  }

  // 删除 cookie
  static delete(name: string): void {
    document.cookie = `${name}=;expires=Thu, 01 Jan 1970 00:00:00 UTC;path=/`
  }

  // 检查 cookie 是否存在
  static exists(name: string): boolean {
    return this.get(name) !== null
  }
}
