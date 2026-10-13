# Learns how much each byte 0x80-0xFF (and CR LF line ends) points to Amiga or PC,
# from files whose machine is known. Trains on half, tests on the other half.
import os, math, hashlib, sys
root='/tmp/bigcheck/files'
def sauce_font(b):
    if len(b)>=128 and b[-128:-121]==b'SAUCE00': return b[-22:].rstrip(b'\0 ').decode('latin1').lower()
    return None
data=[]
for g in os.listdir(root):
    for dp,_,fs in os.walk(os.path.join(root,g)):
        for f in fs:
            p=os.path.join(dp,f)
            if os.path.islink(p) or f.startswith('._'): continue
            ext=f.lower().rsplit('.',1)[-1]
            if ext in ('seq','c','petmate','prg','xb','bin'): continue
            b=open(p,'rb').read()
            if not b or b[:5].lower()==b'<!doc': continue
            sf=sauce_font(b); lab=None
            if sf and sf.startswith('amiga'): lab=1
            elif sf and sf.startswith('ibm'): lab=0
            elif g in ('asciiarena','bbsstyle'): lab=1
            if lab is None: continue
            if sf is not None: b=b[:-128]
            data.append((p,b,lab))
def feats(b):
    n=max(1,len(b)); h=[0]*256
    for c in b: h[c]+=1
    lf=b.count(b'\n'); crlf=b.count(b'\r\n')
    return h, (crlf/lf if lf else 0.5), lf
train=[d for d in data if int(hashlib.md5(d[0].encode()).hexdigest(),16)%2==0]
test=[d for d in data if int(hashlib.md5(d[0].encode()).hexdigest(),16)%2==1]
print('files: amiga', sum(1 for d in data if d[2]==1), 'pc', sum(1 for d in data if d[2]==0), '| train', len(train), 'test', len(test))
# per-file presence-weighted frequency: each file adds its share of high bytes (so big files do not dominate)
A=[1.0]*128; P=[1.0]*128; na=np=0
for p,b,lab in train:
    h,_,_=feats(b); hi=sum(h[128:])
    if hi==0: continue
    for i in range(128):
        v=h[128+i]/hi
        if lab: A[i]+=v
        else: P[i]+=v
    if lab: na+=1
    else: np+=1
sa=sum(A); sp=sum(P)
w=[math.log((A[i]/sa)/(P[i]/sp)) for i in range(128)]
def score(b):
    h,crlf,lf=feats(b); hi=sum(h[128:])
    s=0.0
    if hi: s=sum(h[128+i]/hi*w[i] for i in range(128))*min(hi,200)/20
    # line ends: weak evidence only
    if lf>4: s+= 0.5 if crlf<0.25 else -0.3
    return s
def acc(rows, f):
    ok=sum(1 for p,b,lab in rows if (f(b)>=0)==bool(lab)); return ok, len(rows)
print('learned, on the test half: %d of %d right' % acc(test, score))
amA=[r for r in test if r[2]==1]; pcA=[r for r in test if r[2]==0]
print('  amiga files right: %d of %d, pc files right: %d of %d' % (acc(amA,score)[0],len(amA),acc(pcA,score)[0],len(pcA)))
open('/tmp/bigcheck/learn/weights.txt','w').write(' '.join('%.3f'%x for x in w))
top=sorted(range(128),key=lambda i:w[i])
print('most PC:', [(hex(128+i), round(w[i],2)) for i in top[:12]])
print('most Amiga:', [(hex(128+i), round(w[i],2)) for i in top[-12:]])

# The rules CharEdit uses now (core.go guessAmiga), for comparison.
def old(b):
    pc=am=cr=lf=0.0
    for i,c in enumerate(b):
        if (0xB0<=c<=0xB2) or (0xC0<=c<=0xDF) or c in (0xFE,0xF9,0xFA): pc+=1
        elif c in (0xAF,0xB8,0xB7,0xB4,0xA8,0xA6,0xAB,0xBB): am+=1
        elif c==0x9B and i+1<len(b) and chr(b[i+1]) in '0123456789;mHJK': am+=20
        elif 0x80<=c<0xA0 and c!=0x9B: pc+=3
        elif c==0xFD: pc+=1
        elif c==0x0A:
            lf+=1
            if i>0 and b[i-1]==0x0D: cr+=1
        elif c==0x0C: am+=5
    if lf>4:
        if cr<lf/4: am+=lf/2
        else: pc+=lf/4
    return 1 if am>=pc else -1
print('old rules, on the test half: %d of %d right' % acc(test, old))
print('  amiga files right: %d of %d, pc files right: %d of %d' % (acc(amA,old)[0],len(amA),acc(pcA,old)[0],len(pcA)))
# Combined: the learned score plus the Amiga CSI (0x9B + digits) sign, which is certain.
import re
def combined(b):
    s=score(b)
    if re.search(rb'\x9b[0-9;]*[mHJK]', b): s+=10
    if b.count(b'\x0c')>0 and b'\x1b[' not in b: s+=1
    return s
print('learned + CSI rule: %d of %d right' % acc(test, combined))
print('  amiga files right: %d of %d, pc files right: %d of %d' % (acc(amA,combined)[0],len(amA),acc(pcA,combined)[0],len(pcA)))

miss=[(p,b) for p,b,lab in amA if combined(b)<0]
import collections
grp=collections.Counter(p.split('/files/')[1].split('/')[0] for p,b in miss)
print('missed Amiga files by source:', grp)
blocky=sum(1 for p,b in miss if sum(1 for c in b if c in (0xDB,0xDC,0xDF,0xB0,0xB1,0xB2))>20)
print('of them, drawn with many PC block characters:', blocky)
for p,b in miss[:8]:
    hi=collections.Counter(c for c in b if c>=0x80)
    print(p.split('/files/')[1][:55].ljust(55), [(hex(k),v) for k,v in hi.most_common(5)], 'sauce' if b[-0:] else '')

nos=[r for r in test if r[0].split('/files/')[1].split('/')[0] in ('asciiarena','bbsstyle') and sauce_font(open(r[0],'rb').read()) is None]
print('files without SAUCE (asciiarena.se, BBS styles), test half: old %d, learned %d of %d' % (acc(nos,old)[0], acc(nos,combined)[0], len(nos)))
