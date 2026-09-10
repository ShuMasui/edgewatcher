package com.edgewatcher.di;

import com.edgewatcher.domain.model.DeviceInfo;
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
public final class AppModule_DeviceInfoFactory implements Factory<DeviceInfo> {
  @Override
  public DeviceInfo get() {
    return deviceInfo();
  }

  public static AppModule_DeviceInfoFactory create() {
    return InstanceHolder.INSTANCE;
  }

  public static DeviceInfo deviceInfo() {
    return Preconditions.checkNotNullFromProvides(AppModule.INSTANCE.deviceInfo());
  }

  private static final class InstanceHolder {
    static final AppModule_DeviceInfoFactory INSTANCE = new AppModule_DeviceInfoFactory();
  }
}
