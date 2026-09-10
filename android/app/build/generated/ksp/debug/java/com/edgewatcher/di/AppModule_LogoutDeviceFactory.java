package com.edgewatcher.di;

import com.edgewatcher.domain.port.CredentialStore;
import com.edgewatcher.domain.port.ObservationApi;
import com.edgewatcher.domain.port.ObservationBuffer;
import com.edgewatcher.usecase.LogoutDeviceUseCase;
import dagger.internal.DaggerGenerated;
import dagger.internal.Factory;
import dagger.internal.Preconditions;
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
public final class AppModule_LogoutDeviceFactory implements Factory<LogoutDeviceUseCase> {
  private final Provider<ObservationApi> apiProvider;

  private final Provider<CredentialStore> storeProvider;

  private final Provider<ObservationBuffer> bufferProvider;

  private AppModule_LogoutDeviceFactory(Provider<ObservationApi> apiProvider,
      Provider<CredentialStore> storeProvider, Provider<ObservationBuffer> bufferProvider) {
    this.apiProvider = apiProvider;
    this.storeProvider = storeProvider;
    this.bufferProvider = bufferProvider;
  }

  @Override
  public LogoutDeviceUseCase get() {
    return logoutDevice(apiProvider.get(), storeProvider.get(), bufferProvider.get());
  }

  public static AppModule_LogoutDeviceFactory create(Provider<ObservationApi> apiProvider,
      Provider<CredentialStore> storeProvider, Provider<ObservationBuffer> bufferProvider) {
    return new AppModule_LogoutDeviceFactory(apiProvider, storeProvider, bufferProvider);
  }

  public static LogoutDeviceUseCase logoutDevice(ObservationApi api, CredentialStore store,
      ObservationBuffer buffer) {
    return Preconditions.checkNotNullFromProvides(AppModule.INSTANCE.logoutDevice(api, store, buffer));
  }
}
