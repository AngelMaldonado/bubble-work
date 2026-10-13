export function formatEdit(doc:string,from:number,to:number,marker:string,fallback='texto') {
 const selected=doc.slice(from,to), m=marker.length;
 if(selected.startsWith(marker)&&selected.endsWith(marker)&&selected.length>=m*2) {
  const text=selected.slice(m,-m); return {from,to,text,anchor:from,head:from+text.length};
 }
 if(from>=m&&doc.slice(from-m,from)===marker&&doc.slice(to,to+m)===marker&&selected) {
  return {from:from-m,to:to+m,text:selected,anchor:from-m,head:to-m};
 }
 const text=selected||fallback;
 return {from,to,text:marker+text+marker,anchor:from+m,head:from+m+text.length};
}
