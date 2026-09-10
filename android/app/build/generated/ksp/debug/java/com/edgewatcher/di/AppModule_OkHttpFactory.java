package com.edgewatcher.di;

import dagger.internal.DaggerGenerated;
import dagger.internal.Factory;
import dagger.internal.Preconditions;
import dagger.internal.QualifierMetadata;
import dagger.internal.ScopeMetadata;
import javax.annotation.processing.Generated;
import okhttp3.OkHttpClient;

@ScopeMetadata("javax.inject.Singleton")
@QualifierMetadata
@DaggerGenerated
@Generated(
    value = "dagger.internal.codegen.ComponentProcessor",
    comments = "https://dagger.dev"
)
@SuppressWarnings({
    "unchecked",
    "rawtypes",
    "KotlinInternal",
    "KotlinInternalInJava",
    "cast",
    "deprecation",
    "nullness:initialization.field.uninitialized"
})
public final class AppModule_OkHttpFactory implements Factory<OkHttpClient> {
  @Override
  public OkHttpClient get() {
    return okHttp();
  }

  public static AppModule_OkHttpFactory create() {
    return InstanceHolder.INSTANCE;
  }

  public static OkHttpClient okHttp() {
    return Preconditions.checkNotNullFromProvides(AppModule.INSTANCE.okHttp());
  }

  private static final class InstanceHolder {
    static final AppModule_OkHttpFactory INSTANCE = new AppModule_OkHttpFactory();
  }
}
