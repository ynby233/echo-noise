#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
go install golang.org/x/mobile/cmd/gomobile@latest
go install golang.org/x/mobile/cmd/gobind@latest
export PATH="$HOME/go/bin:$PATH"
go get golang.org/x/mobile/bind
mkdir -p mobile/android/app/libs
gomobile bind -target=android -androidapi 24 -javapkg=cn.noisework.saynote.go -o mobile/android/app/libs/backend.aar ./mobilebackend
  ls -la mobile/android/app/libs
  
  # Dynamically detect the Go Backend class name from the AAR
  echo "Detecting Go Backend class..."
  unzip -p mobile/android/app/libs/backend.aar classes.jar > mobile/android/app/libs/classes.jar
  # Find class file that ends with "Mobilebackend.class" (from package name) or "Backend.class" (fallback)
  CLASS_PATH=$(jar tf mobile/android/app/libs/classes.jar | grep -E "Mobilebackend.class|Backend.class" | head -n 1)
  
  if [ -z "$CLASS_PATH" ]; then
    echo "ERROR: Could not find Go Backend class in AAR. Listing all classes:"
    jar tf mobile/android/app/libs/classes.jar
    exit 1
  fi
  
  FULL_CLASS_NAME=$(echo "$CLASS_PATH" | sed 's/\.class$//' | sed 's/\//./g')
  echo "Detected Go Backend class: $FULL_CLASS_NAME"
  rm mobile/android/app/libs/classes.jar

  PKG_DIR="mobile/android/app/src/main/java/cn/noisework/saynote"
  mkdir -p "$PKG_DIR"
  
  # Generate ServerStarter.java with dynamic import
  cat > "$PKG_DIR/ServerStarter.java" << EOF
package cn.noisework.saynote;
import android.app.Activity;
import android.content.Context;
import android.content.res.AssetManager;
import java.io.File;
import java.io.InputStream;
import java.io.OutputStream;
import java.io.FileOutputStream;
import ${FULL_CLASS_NAME};

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
        // Extract simple class name for the call
        ${FULL_CLASS_NAME##*.}.start(filesDir.getAbsolutePath());
    } catch (Exception e) {
        e.printStackTrace();
    }
    started=true;
  }
EOF

  # Append helper methods to ServerStarter.java (using cat >> to avoid heredoc nesting complexity)
  cat >> "$PKG_DIR/ServerStarter.java" << 'EOF'
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
  # Use fileTree to automatically include all AARs in libs, which is more robust
  perl -0777 -pe 'if(!/fileTree.*libs/){s/dependencies\s*\{(\s*)/dependencies{$1    implementation fileTree(dir: "libs", include: ["*.aar"])\n/s}' -i "$BUILD_GRADLE"
  # Print build.gradle to verify injection in CI logs
  echo "Modified build.gradle content:"
  cat "$BUILD_GRADLE"
fi
