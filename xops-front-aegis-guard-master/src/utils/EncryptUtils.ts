// add 2024-01-16
import JSEncrypt from 'jsencrypt'

let RASkey=`-----BEGIN PUBLIC KEY-----
MIIBCgKCAQEAw8Scd7MILCEhuMcum+5I4ZjRydw02mXYwzXKQlJ3J7Cjs3p9A3Ps
Ys/7bYAdYHMbr8Y/15h0NB5KePUbtwFEFpR5O4eFQiWVBqfeaqV5LH0JJkayRSPP
oUPUHkwsm7BGB25PlPa+JcKCh3VoJqCMWH9nq7aW4DU7UOP5RV5BV4o6+xPTdXys
PMFlqHfU6RH3lcT91ThFNBV1Zn6wOD7fzA7ERtrwFiwu3WuUXdjkMfvHlH2w2ASy
kKfEJpI/D/ufeCUxfIW1vxM3Ppp+KuFJDqBZfNcgUB/7RdrAivu0LaCALChpxpEn
JVL5cnQUY5snAWfgK2o/xzwy9X1IdpPq8wIDAQAB
-----END PUBLIC KEY-----`;

export function encryption(cont) {
  if (cont.length > 240) { // 超长的不做处理 2024-01-16
    return undefined;
  }
  // console.log(` in rsaEncode [${cont}] ... `);
  const encryptor = new JSEncrypt()  // 创建加密对象实例
  //之前ssl生成的公钥，复制的时候要小心不要有空格
  encryptor.setPublicKey(RASkey)//设置公钥
  let rsaCont = encryptor.encrypt(cont)  // 对内容进行加密
  // console.log(` out rsaEncode [${rsaCont}] ... `);
  return rsaCont;
}
