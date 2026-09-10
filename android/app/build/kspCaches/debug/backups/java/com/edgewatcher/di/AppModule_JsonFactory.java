package com.edgewatcher.di;

import dagger.internal.DaggerGenerated;
import dagger.internal.Factory;
import dagger.internal.Preconditions;
import dagger.internal.QualifierMetadata;
import dagger.internal.ScopeMetadata;
import javax.annotation.processing.Generated;
import kotlinx.serialization.json.Json;

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
public final class AppModule_JsonFactory implements Factory<Json> {
  @Override
  public Json get() {
    return json();
  }

  public static AppModule_JsonFactory create() {
    return InstanceHolder.INSTANCE;
  }

  public static Json json() {
    return Preconditions.checkNotNullFromProvides(AppModule.INSTANCE.json());
  }

  private static final class InstanceHolder {
    static final AppModule_JsonFactory INSTANCE = new AppModule_JsonFactory();
  }
}
