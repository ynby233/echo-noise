#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
go install golang.org/x/mobile/cmd/gomobile@latest
go install golang.org/x/mobile/cmd/gobind@latest
export PATH="$HOME/go/bin:$PATH"
go get golang.org/x/mobile/bind
mkdir -p mobile/android/app/libs
gomobile bind -target=android -androidapi 24 -o mobile/android/app/libs/backend.aar ./mobilebackend
PKG_DIR="mobile/android/app/src/main/java/cn/noisework/saynote"
mkdir -p "$PKG_DIR"
cat > "$PKG_DIR/ServerStarter.java" << 'EOF'
package cn.noisework.saynote;
import android.app.Activity;
import android.content.Context;
import android.content.res.AssetManager;
import java.io.File;
import java.io.InputStream;
import java.io.OutputStream;
import java.io.FileOutputStream;
import backend.Backend;
public class ServerStarter {
  private static boolean started=false;
  public static void start(Activity activity){
    if(started)return;
    Context ctx=activity.getApplicationContext();
    File filesDir=ctx.getFilesDir();
    File configDir=new File(filesDir,"config");
    File dataDir=new File(filesDir,"data");
    configDir.mkdirs();
    dataDir.mkdirs();
    copyAssetDir(ctx.getAssets(),"config",configDir);
    copyAssetFile(ctx.getAssets(),"data/noise.db",new File(dataDir,"noise.db"));
    try {
        Backend.start(filesDir.getAbsolutePath());
    } catch (Exception e) {
        e.printStackTrace();
    }
    started=true;
  }
  private static void copyAssetDir(AssetManager am,String assetDir,File outDir){
    try{
      String[] list=am.list(assetDir);
      if(list==null)return;
      for(String name:list){
        String p=assetDir+"/"+name;
        String[] sub=am.list(p);
        if(sub!=null&&sub.length>0){
          File nd=new File(outDir,name);
          nd.mkdirs();
          copyAssetDir(am,p,nd);
        }else{
          copyAssetFile(am,p,new File(outDir,name));
        }
      }
    }catch(Exception ignored){}
  }
  private static void copyAssetFile(AssetManager am,String assetPath,File outFile){
    try{
      if(outFile.exists()&&outFile.length()>0)return;
      InputStream in=am.open(assetPath);
      OutputStream out=new FileOutputStream(outFile);
      byte[] buf=new byte[8192];
      int n;
      while((n=in.read(buf))>0){out.write(buf,0,n);}
      in.close();out.close();
    }catch(Exception ignored){}
  }
}
EOF
MAIN_ACT="mobile/android/app/src/main/java/cn/noisework/saynote/MainActivity.java"
if [ -f "$MAIN_ACT" ]; then
  perl -0777 -pe 's/super\.onCreate\(savedInstanceState\);/super.onCreate(savedInstanceState);\n    ServerStarter.start(this);/s' -i "$MAIN_ACT"
fi
BUILD_GRADLE="mobile/android/app/build.gradle"
if [ -f "$BUILD_GRADLE" ]; then
  perl -0777 -pe 'if(!/implementation files\(\\'\''libs\/backend\.aar\\'\''\)/){s/dependencies\s*\{(\s*)/dependencies{$1    implementation files('\''libs\/backend.aar'')\n/s}' -i "$BUILD_GRADLE"
fi
