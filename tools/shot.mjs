import http from 'http';
import crypto from 'crypto';
const [, , url, out, waitMs] = process.argv;
const wait = parseInt(waitMs || '3000', 10);
function get(path){return new Promise((res,rej)=>{http.get({host:'localhost',port:9333,path},r=>{let b='';r.on('data',c=>b+=c);r.on('end',()=>res(b));}).on('error',rej);});}
function wsConnect(wsUrl){return new Promise((resolve,reject)=>{const key=crypto.randomBytes(16).toString('base64');const u=new URL(wsUrl);const req=http.request({host:u.hostname,port:u.port,path:u.pathname+u.search,headers:{Connection:'Upgrade',Upgrade:'websocket','Sec-WebSocket-Key':key,'Sec-WebSocket-Version':13}});req.on('upgrade',(res,socket)=>resolve(socket));req.on('error',reject);req.end();});}
function frame(data){const payload=Buffer.from(data);const len=payload.length;const mask=crypto.randomBytes(4);let header;if(len<126)header=Buffer.from([0x81,0x80|len]);else if(len<65536){header=Buffer.alloc(4);header[0]=0x81;header[1]=0x80|126;header.writeUInt16BE(len,2);}else{header=Buffer.alloc(10);header[0]=0x81;header[1]=0x80|127;header.writeBigUInt64BE(BigInt(len),2);}const masked=Buffer.from(payload);for(let i=0;i<masked.length;i++)masked[i]^=mask[i%4];return Buffer.concat([header,mask,masked]);}
function makeReader(s,on){let buf=Buffer.alloc(0);s.on('data',d=>{buf=Buffer.concat([buf,d]);while(true){if(buf.length<2)return;const l0=buf[1]&0x7f;let off=2,len=l0;if(l0===126){if(buf.length<4)return;len=buf.readUInt16BE(2);off=4;}else if(l0===127){if(buf.length<10)return;len=Number(buf.readBigUInt64BE(2));off=10;}if(buf.length<off+len)return;const p=buf.slice(off,off+len);buf=buf.slice(off+len);try{on(JSON.parse(p.toString()));}catch(e){}}});}
const targets=JSON.parse(await get('/json/list'));
let page=targets.find(t=>t.type==='page');
if(!page){page=JSON.parse(await get('/json/new?about:blank'));}
const socket=await wsConnect(page.webSocketDebuggerUrl);
let id=0;const pending={};
const send=(m,p={})=>{const i=++id;socket.write(frame(JSON.stringify({id:i,method:m,params:p})));return new Promise(r=>pending[i]=r);};
makeReader(socket,m=>{if(m.id&&pending[m.id]){pending[m.id](m);delete pending[m.id];}});
await send('Page.enable');await send('Network.enable');await send('Network.setCacheDisabled',{cacheDisabled:true});
await send('Emulation.setDeviceMetricsOverride',{width:1280,height:1600,deviceScaleFactor:2,mobile:false});
await send('Page.navigate',{url:'about:blank'});
await new Promise(r=>setTimeout(r,300));
await send('Page.navigate',{url});
await new Promise(r=>setTimeout(r,wait));
const shot=await send('Page.captureScreenshot',{format:'png'});
const b64=shot.result&&shot.result.data;
const { writeFileSync }=await import('fs');
writeFileSync(out,Buffer.from(b64,'base64'));
console.log('saved',out);
socket.end();process.exit(0);
