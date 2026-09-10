package com.edgewatcher.presentation.running;

import com.edgewatcher.usecase.LogoutDeviceUseCase;
import dagger.internal.DaggerGenerated;
import dagger.internal.Factory;
import dagger.internal.Provider;
import dagger.internal.QualifierMetadata;
import dagger.internal.ScopeMetadata;
import javax.annotation.processing.Generated;

@ScopeMetadata
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
public final class RunningViewModel_Factory implements Factory<RunningViewModel> {
  private final Provider<LogoutDeviceUseCase> logoutDeviceProvider;

  private RunningViewModel_Factory(Provider<LogoutDeviceUseCase> logoutDeviceProvider) {
    this.logoutDeviceProvider = logoutDeviceProvider;
  }

  @Override
  public RunningViewModel get() {
    return newInstance(logoutDeviceProvider.get());
  }

  public static RunningViewModel_Factory create(
      Provider<LogoutDeviceUseCase> logoutDeviceProvider) {
    return new RunningViewModel_Factory(logoutDeviceProvider);
  }

  public static RunningViewModel newInstance(LogoutDeviceUseCase logoutDevice) {
    return new RunningViewModel(logoutDevice);
  }
}
