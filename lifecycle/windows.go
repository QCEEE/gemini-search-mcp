package lifecycle

import (
 "os"
 "path/filepath"
 "unsafe"
 "golang.org/x/sys/windows"
)

func cryptData(raw []byte,encrypt bool)([]byte,error){
 if len(raw)==0{return nil,os.ErrInvalid}
 in:=windows.DataBlob{Size:uint32(len(raw)),Data:&raw[0]};var out windows.DataBlob;var err error
 if encrypt {err=windows.CryptProtectData(&in,nil,nil,0,nil,windows.CRYPTPROTECT_UI_FORBIDDEN,&out)} else {err=windows.CryptUnprotectData(&in,nil,nil,0,nil,windows.CRYPTPROTECT_UI_FORBIDDEN,&out)}
 if err!=nil{return nil,err};defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
 plain:=unsafe.Slice(out.Data,int(out.Size));result:=append([]byte(nil),plain...);clear(plain);return result,nil
}
func protect(raw []byte)([]byte,error){return cryptData(raw,true)}
func unprotect(raw []byte)([]byte,error){return cryptData(raw,false)}

func atomicWrite(path string,raw []byte)error{
 f,err:=os.CreateTemp(filepath.Dir(path),".atomic-*");if err!=nil{return err};name:=f.Name();defer os.Remove(name)
 if _,err=f.Write(raw);err!=nil{f.Close();return err};if err=f.Sync();err!=nil{f.Close();return err};if err=f.Close();err!=nil{return err}
 return os.Rename(name,path)
}

func lockLedger(path string)(*os.File,bool,error){
 f,err:=os.OpenFile(path,os.O_RDWR|os.O_CREATE|os.O_EXCL,0600);fresh:=err==nil
 if os.IsExist(err){f,err=os.OpenFile(path,os.O_RDWR,0600)};if err!=nil{return nil,false,err}
 var overlap windows.Overlapped
 if err=windows.LockFileEx(windows.Handle(f.Fd()),windows.LOCKFILE_EXCLUSIVE_LOCK,0,1,0,&overlap);err!=nil{f.Close();return nil,false,err}
 return f,fresh,nil
}
