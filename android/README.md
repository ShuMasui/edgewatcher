# android

EdgeWatcher の Android アプリモジュール。

## ビルド前提条件

- **JAVA_HOME を Android Studio 同梱の JBR 21 に向けること。**
  PATH 上の `java` が Homebrew JDK 26 などの場合、AGP がサポートしていないため
  ビルドが失敗する。

  ```bash
  export JAVA_HOME="/Applications/Android Studio.app/Contents/jbr/Contents/Home"
  ```

- `local.properties` はマシンローカルな設定（`sdk.dir` など）であり、
  リポジトリにはコミットしない（`.gitignore` 済み）。各自の Android SDK の
  インストール先に合わせて作成すること。

  ```properties
  sdk.dir=/path/to/Android/sdk
  ```

## ビルド

```bash
cd android
export JAVA_HOME="/Applications/Android Studio.app/Contents/jbr/Contents/Home"
./gradlew :app:assembleDebug
```
