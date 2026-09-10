package com.edgewatcher.di;

import com.edgewatcher.domain.port.Clock;
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
public final class AppModule_ClockFactory implements Factory<Clock> {
  @Override
  public Clock get() {
    return clock();
  }

  public static AppModule_ClockFactory create() {
    return InstanceHolder.INSTANCE;
  }

  public static Clock clock() {
    return Preconditions.checkNotNullFromProvides(AppModule.INSTANCE.clock());
  }

  private static final class InstanceHolder {
    static final AppModule_ClockFactory INSTANCE = new AppModule_ClockFactory();
  }
}
