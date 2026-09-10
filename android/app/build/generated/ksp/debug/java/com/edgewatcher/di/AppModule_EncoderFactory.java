package com.edgewatcher.di;

import com.edgewatcher.domain.port.JpegEncoder;
import dagger.internal.DaggerGenerated;
import dagger.internal.Factory;
import dagger.internal.Preconditions;
import dagger.internal.QualifierMetadata;
import dagger.internal.ScopeMetadata;
import javax.annotation.processing.Generated;

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
public final class AppModule_EncoderFactory implements Factory<JpegEncoder> {
  @Override
  public JpegEncoder get() {
    return encoder();
  }

  public static AppModule_EncoderFactory create() {
    return InstanceHolder.INSTANCE;
  }

  public static JpegEncoder encoder() {
    return Preconditions.checkNotNullFromProvides(AppModule.INSTANCE.encoder());
  }

  private static final class InstanceHolder {
    static final AppModule_EncoderFactory INSTANCE = new AppModule_EncoderFactory();
  }
}
